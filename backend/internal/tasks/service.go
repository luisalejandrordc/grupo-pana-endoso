package tasks

import (
	"context"
	"encoding/json"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/outbox"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	errForbidden       = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
	errSaleNotFound    = apperr.NotFound("SALE_NOT_FOUND", "La venta no existe.")
	errTaskNotFound    = apperr.NotFound("TASK_NOT_FOUND", "La tarea no existe.")
	errVersionRequired = apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	errVersionConflict = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó esta tarea. Recargue e intente nuevamente.")
	errReasonRequired  = apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo.").
				WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio."})
	errSaleNotActive = apperr.Conflict("SALE_NOT_ACTIVE", "La venta no está activa: sus tareas no se pueden modificar.")
	errNotEligible   = apperr.Conflict("TASK_NOT_ELIGIBLE", "Esta tarea aún no se puede iniciar: la anterior debe estar completada y no puede haber otra en curso.")
	errNotInProgress = apperr.Conflict("TASK_NOT_IN_PROGRESS", "La tarea no está en curso.")
	errNotCompleted  = apperr.Conflict("TASK_NOT_COMPLETED", "La tarea no está completada.")
	errReadOnly      = apperr.Conflict("TASK_READ_ONLY", "La tarea aún no está en curso: sus datos no se pueden editar.")
	errCannotReopen  = apperr.Conflict("TASK_REOPEN_NOT_ALLOWED", "Solo se puede reabrir la última tarea completada, y solo si no hay otra en curso.")
)

func lockedError(fields ...string) *apperr.Error {
	e := apperr.Conflict("TASK_FINANCIAL_FIELD_LOCKED", "Este monto ya no se puede modificar porque una tarea posterior que depende de él ya está completada.")
	for _, f := range fields {
		e = e.WithFields(apperr.FieldError{Field: f, Code: "LOCKED", Message: "Bloqueado: una tarea posterior ya se completó."})
	}
	return e
}

// Deps are the engine's dependencies.
type Deps struct {
	DB     auth.Database
	Logger *slog.Logger
	Now    func() time.Time // nil means time.Now
}

// Service runs the task operations of a sale.
type Service struct {
	db     auth.Database
	logger *slog.Logger
	now    func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, logger: d.Logger, now: d.Now}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func isStaff(a auth.Actor) bool {
	return a.Role == auth.RoleSystemAdmin || a.Role == auth.RoleSalesManager || a.Role == auth.RoleSalesAdvisor
}

// access loads the sale for an actor. A sale an Advisor does not own does not exist for them. With lock set
// the row is locked, which serializes every change to one sale.
func (s *Service) access(ctx context.Context, q querier, actor auth.Actor, saleID uuid.UUID, lock bool) (*SaleRow, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	read := ReadSale
	if lock {
		read = LockSale
	}
	sale, err := read(ctx, q, saleID)
	if err != nil {
		return nil, err
	}
	if sale == nil || (actor.Role == auth.RoleSalesAdvisor && sale.AdvisorID != actor.ID) {
		return nil, errSaleNotFound
	}
	return sale, nil
}

func own(actor auth.Actor, sale *SaleRow) bool { return sale.AdvisorID == actor.ID }

func validTask(n int) error {
	if n < 1 || n > TaskCount {
		return errTaskNotFound
	}
	return nil
}

// --- reading ---

func summarize(rows []taskRow, defs map[int]definition) []Summary {
	statuses := statusesOf(rows)
	out := make([]Summary, len(rows))
	for i, r := range rows {
		d := defs[r.Number]
		out[i] = Summary{
			Number: r.Number, Name: d.name, MilestoneNumber: d.milestone, Status: r.Status, StartedAt: r.StartedAt,
			CompletedAt: r.CompletedAt, DueDate: r.DueDate, CompletionPhase: r.CompletionPhase, RecommendedDurationHours: d.durationHours,
			IsEligible: IsEligible(statuses, r.Number), Version: r.Version,
		}
	}
	return out
}

// Summaries returns the 13 tasks of a sale. The sales package uses it to build the sale detail.
func Summaries(ctx context.Context, q Querier, saleID uuid.UUID) ([]Summary, error) {
	rows, err := loadTaskRows(ctx, q, saleID)
	if err != nil {
		return nil, err
	}
	defs, err := loadDefinitions(ctx, q)
	if err != nil {
		return nil, err
	}
	return summarize(rows, defs), nil
}

// List returns the 13 tasks of a sale the actor can see.
func (s *Service) List(ctx context.Context, actor auth.Actor, saleID uuid.UUID) ([]Summary, error) {
	if _, err := s.access(ctx, s.db, actor, saleID, false); err != nil {
		return nil, err
	}
	return Summaries(ctx, s.db, saleID)
}

// Get returns one task with its data, calculated values, documents, warnings and available actions.
func (s *Service) Get(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	sale, err := s.access(ctx, s.db, actor, saleID, false)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, s.db, actor, sale, n)
}

func (s *Service) detail(ctx context.Context, q querier, actor auth.Actor, sale *SaleRow, n int) (*Detail, error) {
	rows, err := loadTaskRows(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	defs, err := loadDefinitions(ctx, q)
	if err != nil {
		return nil, err
	}
	data, err := loadData(ctx, q, sale.ID, n)
	if err != nil {
		return nil, err
	}
	f, err := loadFin(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	row := rows[n-1]
	docs, err := loadDocuments(ctx, q, row.ID, n, sale.CustomerType)
	if err != nil {
		return nil, err
	}
	warnings, err := s.warnings(ctx, q, sale, n, nil)
	if err != nil {
		return nil, err
	}
	d := &Detail{
		Summary: summarize(rows, defs)[n-1], SaleID: sale.ID, Notes: row.Notes, Data: data,
		Calculated: calculated(n, data, f), Documents: docs, Warnings: warnings,
	}
	d.Permissions = permissions(actor, sale, rows, n)
	return d, nil
}

func permissions(actor auth.Actor, sale *SaleRow, rows []taskRow, n int) Permissions {
	statuses := statusesOf(rows)
	row := rows[n-1]
	acts := CanActOnTask(actor.Role, own(actor, sale))
	management := IsManagement(actor.Role)
	active := sale.State == "active"
	notCancelled := sale.State != "cancelled"
	completed := CompletedCount(statuses)
	hasInProgress := CurrentTask(statuses) != 0 && statuses[CurrentTask(statuses)-1] == StatusInProgress
	return Permissions{
		CanStart:     acts && active && IsEligible(statuses, n),
		CanComplete:  acts && active && row.Status == StatusInProgress,
		CanEditData:  (active && acts && row.Status == StatusInProgress) || (notCancelled && management && row.Status == StatusCompleted),
		NeedsReason:  row.Status == StatusCompleted,
		CanSaveNotes: acts && notCancelled,
		CanRestart:   management && active && row.Status == StatusInProgress,
		CanReopen: management && notCancelled && row.Status == StatusCompleted && n == completed && !hasInProgress &&
			(sale.State == "active" || n == TaskCount),
	}
}

// taskGroups says which chronology groups belong to which task, so a task shows only its own warnings.
var taskGroups = map[int][]int{
	4: {GroupPanaPayment}, 5: {GroupToyotaPayment}, 6: {GroupInvoices}, 10: {GroupRegistration},
	11: {GroupPDSTransfer}, 12: {GroupEquipment}, 13: {GroupSurquillo, GroupDelivery},
}

// warnings computes the chronology remarks for the dates of task n. overrides are dates about to be saved,
// which replace the stored ones.
func (s *Service) warnings(ctx context.Context, q querier, sale *SaleRow, n int, overrides []DateEntry) ([]Warning, error) {
	groups, ok := taskGroups[n]
	if !ok {
		return []Warning{}, nil
	}
	stored, err := loadDates(ctx, q, sale.ID, sale.CreatedAt)
	if err != nil {
		return nil, err
	}
	key := func(e DateEntry) string { return string(rune('a'+e.Group)) + "|" + strings.SplitN(e.Field, "#", 2)[0] }
	merged := map[string]DateEntry{}
	var order []string
	for _, e := range append(stored, overrides...) {
		k := key(e)
		if _, seen := merged[k]; !seen {
			order = append(order, k)
		}
		merged[k] = e
	}
	entries := make([]DateEntry, 0, len(order))
	for _, k := range order {
		entries = append(entries, merged[k])
	}
	today := dateOnly(s.now())
	out := []Warning{}
	for _, w := range ChronologyWarnings(entries, today) {
		for _, e := range entries {
			if e.Field != w.Field {
				continue
			}
			for _, g := range groups {
				if e.Group == g {
					w.Field = strings.SplitN(w.Field, "#", 2)[0]
					out = append(out, w)
				}
			}
			break
		}
	}
	return out, nil
}

// --- transitions ---

// Start moves the eligible task to in progress and sets its due date.
func (s *Service) Start(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if !CanActOnTask(actor.Role, own(actor, sale)) {
			return errForbidden
		}
		if sale.State != "active" {
			return errSaleNotActive
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		if !IsEligible(statusesOf(rows), n) {
			return errNotEligible
		}
		defs, err := loadDefinitions(ctx, tx)
		if err != nil {
			return err
		}
		var due *time.Time
		if h := defs[n].durationHours; h != nil {
			d := DueDate(now, *h)
			due = &d
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'in_progress', started_at = $2, due_date = $3, updated_by = $4,
				updated_at = $2, row_version = row_version + 1 WHERE id = $1`, rows[n-1].ID, now, due, actor.ID); err != nil {
			return err
		}
		if err := touchSale(ctx, tx, saleID, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: rows[n-1].ID,
			Action: audit.ActionStarted, Changes: map[string]any{"taskNumber": n},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.Get(ctx, actor, saleID, n)
}

func touchSale(ctx context.Context, q querier, saleID, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE sale_registers SET updated_by = $2, updated_at = $3 WHERE id = $1`, saleID, actor, now)
	return err
}

// Complete validates the task and marks it completed. Completing task 13 completes the sale.
func (s *Service) Complete(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if !CanActOnTask(actor.Role, own(actor, sale)) {
			return errForbidden
		}
		if sale.State != "active" {
			return errSaleNotActive
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		row := rows[n-1]
		if row.Status != StatusInProgress {
			return errNotInProgress
		}
		data, err := loadData(ctx, tx, saleID, n)
		if err != nil {
			return err
		}
		f, err := loadFin(ctx, tx, saleID)
		if err != nil {
			return err
		}
		docs, err := loadDocuments(ctx, tx, row.ID, n, sale.CustomerType)
		if err != nil {
			return err
		}
		if problems := completionProblems(n, data, f, docs); len(problems) > 0 {
			return apperr.Unprocessable("TASK_INCOMPLETE", "La tarea no se puede completar: faltan datos o documentos.").WithFields(problems...)
		}
		defs, err := loadDefinitions(ctx, tx)
		if err != nil {
			return err
		}
		def := defs[n]
		dur, rem := 0, 0
		if def.durationHours != nil {
			dur, rem = *def.durationHours, *def.reminderHours
		}
		phase := CompletionPhase(dur, rem, *row.StartedAt, now)
		if err := applyWrites(ctx, tx, saleID, actor.ID, expectedWrites(n, f), now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'completed', completed_at = $2, completion_phase = $3::text::completion_phase,
				updated_by = $4, updated_at = $2, row_version = row_version + 1 WHERE id = $1`, row.ID, now, string(phase), actor.ID); err != nil {
			return err
		}
		if err := touchSale(ctx, tx, saleID, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
			Action: audit.ActionCompleted, Changes: map[string]any{"taskNumber": n, "completionPhase": string(phase)},
		}, now); err != nil {
			return err
		}
		if n == TaskCount {
			if _, err := tx.Exec(ctx, `UPDATE sale_registers SET overall_state = 'completed', completed_at = $2 WHERE id = $1`, saleID, now); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntitySale, EntityID: saleID,
				Action: audit.ActionCompleted, Changes: map[string]any{"automatic": true, "reason": "task 13 completed"},
			}, now); err != nil {
				return err
			}
		}
		if kind := outbox.MilestoneTaskType(n); kind != "" {
			contact, err := LoadCustomerContact(ctx, tx, saleID)
			if err != nil {
				return err
			}
			// The same key every time: finishing a task that was reopened never e-mails the customer twice.
			return outbox.Enqueue(ctx, tx, outbox.Notification{
				Type: kind, DedupeKey: kind + ":" + saleID.String(),
				SaleID: &saleID, TaskID: &row.ID, CustomerID: &contact.ID, Email: contact.Email,
			})
		}
		return nil
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.Get(ctx, actor, saleID, n)
}

// Restart puts an in-progress task back to pending (Manager, Admin), clearing its clock and reminders. The
// data and files already saved are kept, read-only.
func (s *Service) Restart(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int, reason string) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	if !isStaff(actor) || !IsManagement(actor.Role) {
		if isStaff(actor) {
			// An Advisor learns nothing about sales they do not own.
			if _, err := s.access(ctx, s.db, actor, saleID, false); err != nil {
				return nil, err
			}
		}
		return nil, errForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errReasonRequired
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if sale.State != "active" {
			return errSaleNotActive
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		row := rows[n-1]
		if row.Status != StatusInProgress {
			return errNotInProgress
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'pending', started_at = NULL, due_date = NULL, completion_phase = NULL,
				approaching_warning_sent_at = NULL, last_reminder_sent_at = NULL, overdue_reminder_count = 0,
				updated_by = $2, updated_at = $3, row_version = row_version + 1 WHERE id = $1`, row.ID, actor.ID, now); err != nil {
			return err
		}
		if err := touchSale(ctx, tx, saleID, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
			Action: audit.ActionRestarted, Reason: reason, Changes: map[string]any{"taskNumber": n},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.Get(ctx, actor, saleID, n)
}

// Reopen turns the last completed task back to in progress (Manager, Admin), keeping its original start time
// and due date. Reopening task 13 of a completed sale makes the sale active again.
func (s *Service) Reopen(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int, reason string) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	if !isStaff(actor) || !IsManagement(actor.Role) {
		if isStaff(actor) {
			if _, err := s.access(ctx, s.db, actor, saleID, false); err != nil {
				return nil, err
			}
		}
		return nil, errForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errReasonRequired
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if sale.State == "cancelled" {
			return errSaleNotActive
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		statuses := statusesOf(rows)
		row := rows[n-1]
		if row.Status != StatusCompleted {
			return errNotCompleted
		}
		if n != CompletedCount(statuses) || (sale.State == "completed" && n != TaskCount) {
			return errCannotReopen
		}
		for _, st := range statuses {
			if st == StatusInProgress {
				return errCannotReopen
			}
		}
		if err := clearExpected(ctx, tx, saleID, n); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'in_progress', completed_at = NULL, completion_phase = NULL,
				updated_by = $2, updated_at = $3, row_version = row_version + 1 WHERE id = $1`, row.ID, actor.ID, now); err != nil {
			return err
		}
		if sale.State == "completed" {
			if _, err := tx.Exec(ctx, `UPDATE sale_registers SET overall_state = 'active', completed_at = NULL WHERE id = $1`, saleID); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntitySale, EntityID: saleID,
				Action: audit.ActionReopened, Reason: reason, Changes: map[string]any{"from": "completed", "to": "active"},
			}, now); err != nil {
				return err
			}
		}
		if err := touchSale(ctx, tx, saleID, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
			Action: audit.ActionReopened, Reason: reason, Changes: map[string]any{"taskNumber": n},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.Get(ctx, actor, saleID, n)
}

// --- saving data ---

// financialLocks lists the fields of task n that cannot change once the task that depends on them is done:
// task 1's amounts once any later task is completed, task 3's final amount once any task after 3 is, and the
// deposits of tasks 4 and 5 once task 6 is.
func financialLocks(n int, changed []string, completed []bool) []string {
	later := func(from int) bool {
		for i := from; i < TaskCount; i++ {
			if completed[i] {
				return true
			}
		}
		return false
	}
	var locked []string
	for _, f := range changed {
		switch {
		case n == 1 && later(1):
			locked = append(locked, f)
		case n == 3 && f == "finalToyotaAmount" && later(3):
			locked = append(locked, f)
		case (n == 4 || n == 5) && f == "depositedAmount" && completed[5]:
			locked = append(locked, f)
		}
	}
	return locked
}

// SaveData validates and saves the fields of a task. While the task is in progress, an Advisor on their own
// sale (or a Manager or Admin) saves a draft; once it is completed, only a Manager or Admin can correct it, with a
// written reason. Impossible values are refused and out-of-order dates only warn.
func (s *Service) SaveData(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int, in SaveInput) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	if in.Version == nil {
		return nil, errVersionRequired
	}
	now := s.now()
	var warnings []Warning
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		row := rows[n-1]
		switch {
		case sale.State == "cancelled":
			return errSaleNotActive
		case row.Status == StatusPending:
			return errReadOnly
		case row.Status == StatusInProgress:
			if !CanActOnTask(actor.Role, own(actor, sale)) {
				return errForbidden
			}
			if sale.State != "active" {
				return errSaleNotActive
			}
		case row.Status == StatusCompleted:
			if !IsManagement(actor.Role) {
				return errForbidden
			}
			if strings.TrimSpace(in.Reason) == "" {
				return errReasonRequired
			}
		}
		if row.Version != *in.Version {
			return errVersionConflict
		}

		current, err := loadData(ctx, tx, saleID, n)
		if err != nil {
			return err
		}
		f, err := loadFin(ctx, tx, saleID)
		if err != nil {
			return err
		}
		p := &planner{today: dateOnly(now)}
		if err := p.plan(n, in.Data, current, f.snapshot()); err != nil {
			return apperr.BadRequest("INVALID_JSON", "Los datos de la tarea no son válidos: "+err.Error()).WithCause(err)
		}
		if p.failed() {
			return apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").WithFields(p.problems...)
		}
		completed := make([]bool, TaskCount)
		for i, r := range rows {
			completed[i] = r.Status == StatusCompleted
		}
		if locked := financialLocks(n, p.changed, completed); len(locked) > 0 {
			return lockedError(locked...)
		}
		if len(p.writes) > 0 {
			if err := applyWrites(ctx, tx, saleID, actor.ID, p.writes, now); err != nil {
				return mapSaveError(err)
			}
		}
		if row.Status == StatusCompleted {
			// A correction must leave the task complete: a mandatory field cannot be cleared, and an amount that
			// differs from the expected one still needs its reason.
			after, err := loadData(ctx, tx, saleID, n)
			if err != nil {
				return err
			}
			f2, err := loadFin(ctx, tx, saleID)
			if err != nil {
				return err
			}
			var problems []apperr.FieldError
			for _, pr := range completionProblems(n, after, f2, nil) {
				problems = append(problems, pr)
			}
			if len(problems) > 0 {
				return apperr.Unprocessable("TASK_INCOMPLETE", "La corrección dejaría la tarea incompleta.").WithFields(problems...)
			}
			changes := diffData(current, after, n == 2)
			if len(changes) > 0 {
				if err := audit.Record(ctx, tx, audit.Event{
					ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
					Action: audit.ActionCorrected, Reason: strings.TrimSpace(in.Reason), Changes: map[string]any{"taskNumber": n, "fields": changes},
				}, now); err != nil {
					return err
				}
			}
		} else if len(p.changed) > 0 {
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
				Action: audit.ActionUpdated, Changes: map[string]any{"taskNumber": n, "fields": p.changed},
			}, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET updated_by = $2, updated_at = $3, row_version = row_version + 1 WHERE id = $1`,
			row.ID, actor.ID, now); err != nil {
			return err
		}
		if err := touchSale(ctx, tx, saleID, actor.ID, now); err != nil {
			return err
		}
		warnings, err = s.warnings(ctx, tx, sale, n, p.dates)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	d, err := s.Get(ctx, actor, saleID, n)
	if err != nil {
		return nil, err
	}
	d.Warnings = warnings
	return d, nil
}

// uniqueFields attributes a uniqueness conflict to the field that caused it.
var uniqueFields = map[string]string{
	"SERIES_ALREADY_USED":         "vehicleSeries",
	"VIN_ALREADY_USED":            "vin",
	"PLATE_ALREADY_USED":          "licensePlate",
	"DAM_ALREADY_USED":            "damNumber",
	"TRANSACTION_ALREADY_USED":    "transactionNumber",
	"MISMATCH_REASON_REQUIRED":    "mismatchReason",
	"INVOICE_NUMBER_ALREADY_USED": "number",
}

func mapSaveError(err error) error {
	e := apperr.FromError(err)
	if field, ok := uniqueFields[e.Code]; ok && len(e.Fields) == 0 {
		return e.WithFields(apperr.FieldError{Field: field, Code: e.Code, Message: e.Message})
	}
	return e
}

// SaveNotes saves the free-text notes of a task.
func (s *Service) SaveNotes(ctx context.Context, actor auth.Actor, saleID uuid.UUID, n int, version *int, notes string) (*Detail, error) {
	if err := validTask(n); err != nil {
		return nil, err
	}
	if version == nil {
		return nil, errVersionRequired
	}
	notes = strings.TrimSpace(notes)
	if len([]rune(notes)) > 2000 {
		return nil, apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").
			WithFields(apperr.FieldError{Field: "notes", Code: "TOO_LONG", Message: "Las notas son demasiado largas (máximo 2000 caracteres)."})
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.access(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if !CanActOnTask(actor.Role, own(actor, sale)) {
			return errForbidden
		}
		if sale.State == "cancelled" {
			return errSaleNotActive
		}
		rows, err := loadTaskRows(ctx, tx, saleID)
		if err != nil {
			return err
		}
		row := rows[n-1]
		if row.Version != *version {
			return errVersionConflict
		}
		var value any
		if notes != "" {
			value = notes
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET notes = $2, updated_by = $3, updated_at = $4, row_version = row_version + 1 WHERE id = $1`,
			row.ID, value, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTask, EntityID: row.ID,
			Action: audit.ActionUpdated, Changes: map[string]any{"taskNumber": n, "fields": []string{"notes"}},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.Get(ctx, actor, saleID, n)
}

// diffData compares two task data objects and returns the fields that changed with their before and after
// values. Personal data (task 2: who signs) is reported as "changed" without the values.
func diffData(before, after any, redact bool) map[string]any {
	flatten := func(v any) map[string]any {
		raw, _ := json.Marshal(v)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		out := map[string]any{}
		var walk func(prefix string, m map[string]any)
		walk = func(prefix string, m map[string]any) {
			for k, val := range m {
				if sub, ok := val.(map[string]any); ok {
					walk(prefix+k+".", sub)
					continue
				}
				out[prefix+k] = val
			}
		}
		walk("", tree)
		return out
	}
	b, a := flatten(before), flatten(after)
	changes := map[string]any{}
	for k, av := range a {
		if bv := b[k]; bv != av {
			if redact {
				changes[k] = "changed"
			} else {
				changes[k] = map[string]any{"before": bv, "after": av}
			}
		}
	}
	return changes
}
