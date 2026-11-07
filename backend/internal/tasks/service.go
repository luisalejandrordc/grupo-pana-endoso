package tasks

import (
	"context"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	errSaleNotFound      = apperr.NotFound("SALE_NOT_FOUND", "La venta no existe.")
	errTaskNotFound      = apperr.NotFound("TASK_NOT_FOUND", "La tarea no existe.")
	errForbidden         = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
	errVersionConflict   = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó este registro. Recargue e intente nuevamente.")
	errSaleNotActive     = apperr.Conflict("SALE_NOT_ACTIVE", "La venta no está activa: sus tareas no se pueden modificar.")
	errInvalidState      = apperr.Conflict("TASK_INVALID_STATE", "La tarea no está en el estado que requiere esta acción. Recargue e intente nuevamente.")
	errNotInProgress     = apperr.Conflict("TASK_NOT_IN_PROGRESS", "La tarea no está en curso: inicie la tarea para editar sus datos.")
	errNotEligible       = apperr.Conflict("TASK_NOT_ELIGIBLE", "La tarea anterior todavía no está completada.")
	errAlreadyInProgress = apperr.Conflict("TASK_ALREADY_IN_PROGRESS", "La venta ya tiene una tarea en curso.")
	errNotLastCompleted  = apperr.Conflict("TASK_NOT_LAST_COMPLETED", "Solo puede reabrirse la última tarea completada.")
	errDocumentsOff      = apperr.New(http.StatusServiceUnavailable, "DOCUMENTS_UNAVAILABLE",
		"La verificación de documentos no está disponible. No se puede completar la tarea.")
)

// Deps are the service's dependencies.
type Deps struct {
	DB        auth.Database
	Sales     SaleGate
	Documents DocumentChecker // nil means document checks are unavailable and no task can be completed
	Notifier  Notifier        // nil means NoopNotifier
	Logger    *slog.Logger
	Now       func() time.Time // nil means time.Now
}

// Service implements the task use cases.
type Service struct {
	db        auth.Database
	sales     SaleGate
	documents DocumentChecker
	notifier  Notifier
	logger    *slog.Logger
	now       func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, sales: d.Sales, documents: d.Documents, notifier: d.Notifier, logger: d.Logger, now: d.Now}
	if s.now == nil {
		s.now = time.Now
	}
	if s.notifier == nil {
		s.notifier = NoopNotifier{}
	}
	return s
}

func isStaff(actor auth.Actor) bool {
	switch actor.Role {
	case auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor:
		return true
	}
	return false
}

func isManagement(actor auth.Actor) bool {
	return actor.Role == auth.RoleSystemAdmin || actor.Role == auth.RoleSalesManager
}

func checkNumber(number int) error {
	if number < 1 || number > LastTask {
		return errTaskNotFound
	}
	return nil
}

// scoped finds the sale and applies the actor's scope: an Advisor only reaches the sales assigned to them,
// and any other sale answers as if it did not exist.
func (s *Service) scoped(ctx context.Context, q Querier, actor auth.Actor, saleID uuid.UUID, forUpdate bool) (*SaleRef, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	sale, err := s.sales.Find(ctx, q, saleID, forUpdate)
	if err != nil {
		return nil, err
	}
	if sale == nil {
		return nil, errSaleNotFound
	}
	if actor.Role == auth.RoleSalesAdvisor && sale.AssignedAdvisorID != actor.ID {
		return nil, errSaleNotFound
	}
	return sale, nil
}

// reason trims and checks the reason of a rollback.
func cleanReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	switch {
	case reason == "":
		return "", apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo.").
			WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio."})
	case utf8.RuneCountInString(reason) > 500:
		return "", apperr.Unprocessable("REASON_TOO_LONG", "El motivo es demasiado largo.").
			WithFields(apperr.FieldError{Field: "reason", Code: "TOO_LONG", Message: "El motivo no puede superar los 500 caracteres."})
	}
	return reason, nil
}

// detail reads one task with its data. The sale is read again by the caller after writes, so identifiers
// that a save changed show up.
func (s *Service) detail(ctx context.Context, q Querier, sale *SaleRef, number int) (*Detail, error) {
	t, err := findTask(ctx, q, sale.ID, number, false)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errTaskNotFound
	}
	if t.Status == StatusPending {
		t.Eligible, err = previousCompleted(ctx, q, sale.ID, number)
		if err != nil {
			return nil, err
		}
	}
	data, err := kindFor(number).load(ctx, q, sale)
	if err != nil {
		return nil, err
	}
	return &Detail{Task: t.Task, Data: data, RequiredDocuments: RequiredSlots(number, sale.CustomerType)}, nil
}

// refreshed returns the sale as it is now, inside the transaction that changed it.
func (s *Service) refreshed(ctx context.Context, q Querier, saleID uuid.UUID) (*SaleRef, error) {
	sale, err := s.sales.Find(ctx, q, saleID, false)
	if err != nil {
		return nil, err
	}
	if sale == nil {
		return nil, errSaleNotFound
	}
	return sale, nil
}

// List returns the 13 tasks of a sale the actor may see, without their data.
func (s *Service) List(ctx context.Context, actor auth.Actor, saleID uuid.UUID) ([]Task, error) {
	if _, err := s.scoped(ctx, s.db, actor, saleID, false); err != nil {
		return nil, err
	}
	rows, err := listTasks(ctx, s.db, saleID)
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, len(rows))
	for i, r := range rows {
		tasks[i] = r.Task
	}
	return tasks, nil
}

// Get returns one task with its data.
func (s *Service) Get(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	sale, err := s.scoped(ctx, s.db, actor, saleID, false)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, s.db, sale, number)
}

// lockedTask applies the common start of every transition: the sale must be active, and the task is locked.
func (s *Service) lockedTask(ctx context.Context, tx pgx.Tx, actor auth.Actor, saleID uuid.UUID, number int) (*SaleRef, *taskRow, error) {
	sale, err := s.scoped(ctx, tx, actor, saleID, true)
	if err != nil {
		return nil, nil, err
	}
	if sale.State != SaleActive {
		return nil, nil, errSaleNotActive
	}
	t, err := findTask(ctx, tx, saleID, number, true)
	if err != nil {
		return nil, nil, err
	}
	if t == nil {
		return nil, nil, errTaskNotFound
	}
	return sale, t, nil
}

func (s *Service) recordTask(ctx context.Context, tx pgx.Tx, actor auth.Actor, saleID uuid.UUID, task uuid.UUID,
	action, reason string, changes map[string]any, now time.Time,
) error {
	return audit.Record(ctx, tx, audit.Event{
		ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntityTaskInstance, EntityID: task,
		Action: action, Reason: reason, Changes: changes,
	}, now)
}

// Start moves an eligible task to in progress and sets its due date when it has a duration (T1).
func (s *Service) Start(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	now := s.now()
	var result *Detail
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, t, err := s.lockedTask(ctx, tx, actor, saleID, number)
		if err != nil {
			return err
		}
		if t.Status != StatusPending {
			return errInvalidState
		}
		eligible, err := previousCompleted(ctx, tx, saleID, number)
		if err != nil {
			return err
		}
		if !eligible {
			return errNotEligible
		}
		busy, err := anyInProgress(ctx, tx, saleID)
		if err != nil {
			return err
		}
		if busy {
			return errAlreadyInProgress
		}
		var due *time.Time
		if t.DurationHours != nil {
			value := dueDateFor(now, *t.DurationHours)
			due = &value
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'in_progress', started_at = $2, due_date = $3,
			completion_phase = NULL, approaching_warning_sent_at = NULL, last_reminder_sent_at = NULL,
			overdue_reminder_count = 0, updated_by = $4, updated_at = $2, row_version = row_version + 1
			WHERE id = $1`, t.ID, now, due, actor.ID); err != nil {
			return err
		}
		if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionStarted, "", map[string]any{"taskNumber": number}, now); err != nil {
			return err
		}
		result, err = s.detail(ctx, tx, sale, number)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// Complete finishes the task in progress when all of its data and documents are valid (T2). Completing
// Task 13 completes the sale in the same transaction (S2).
func (s *Service) Complete(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	if s.documents == nil {
		return nil, errDocumentsOff
	}
	now := s.now()
	var result *Detail
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, t, err := s.lockedTask(ctx, tx, actor, saleID, number)
		if err != nil {
			return err
		}
		if t.Status != StatusInProgress {
			return errInvalidState
		}

		problems, err := kindFor(number).check(ctx, tx, sale, actor.ID, now)
		if err != nil {
			return err
		}
		missing, err := s.documents.MissingSlots(ctx, tx, t.ID, RequiredSlots(number, sale.CustomerType))
		if err != nil {
			return err
		}
		for _, slot := range missing {
			problems = append(problems, apperr.FieldError{
				Field: "documents." + slot, Code: "DOCUMENT_REQUIRED",
				Message: "Falta subir un documento obligatorio de esta tarea.",
			})
		}
		if len(problems) > 0 {
			return apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").WithFields(problems...)
		}

		phase := completionPhase(now, t.DueDate, t.DurationHours, t.intervalHours)
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'completed', completed_at = $2,
			completion_phase = $3::text::completion_phase, updated_by = $4, updated_at = $2, row_version = row_version + 1
			WHERE id = $1`, t.ID, now, phase, actor.ID); err != nil {
			return err
		}
		if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionCompleted, "",
			map[string]any{"taskNumber": number, "completionPhase": phase}, now); err != nil {
			return err
		}
		if number == LastTask {
			if err := s.sales.MarkCompleted(ctx, tx, saleID, actor.ID, now); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntitySaleRegister, EntityID: saleID,
				Action: audit.ActionCompleted, Changes: map[string]any{"automatic": true, "trigger": "task_13_completed"},
			}, now); err != nil {
				return err
			}
		}
		if err := s.notifier.TaskCompleted(ctx, tx, saleID, number, now); err != nil {
			return err
		}
		sale, err = s.refreshed(ctx, tx, saleID)
		if err != nil {
			return err
		}
		result, err = s.detail(ctx, tx, sale, number)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// Restart sends the task in progress back to pending, keeping its data read-only (T4).
func (s *Service) Restart(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int, rawReason string) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	if !isManagement(actor) {
		return nil, errForbidden
	}
	reason, err := cleanReason(rawReason)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var result *Detail
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, t, err := s.lockedTask(ctx, tx, actor, saleID, number)
		if err != nil {
			return err
		}
		if t.Status != StatusInProgress {
			return errInvalidState
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'pending', started_at = NULL, due_date = NULL,
			completion_phase = NULL, approaching_warning_sent_at = NULL, last_reminder_sent_at = NULL,
			overdue_reminder_count = 0, updated_by = $2, updated_at = $3, row_version = row_version + 1
			WHERE id = $1`, t.ID, actor.ID, now); err != nil {
			return err
		}
		if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionRestarted, reason, map[string]any{"taskNumber": number}, now); err != nil {
			return err
		}
		result, err = s.detail(ctx, tx, sale, number)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// Reopen sends the last completed task back to in progress (T3). When the sale was completed it becomes
// active again in the same transaction (S3). The original due date and the reminder counters are kept.
func (s *Service) Reopen(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int, rawReason string) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	if !isManagement(actor) {
		return nil, errForbidden
	}
	reason, err := cleanReason(rawReason)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var result *Detail
	err = database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.scoped(ctx, tx, actor, saleID, true)
		if err != nil {
			return err
		}
		if sale.State == SaleCancelled {
			return errSaleNotActive
		}
		t, err := findTask(ctx, tx, saleID, number, true)
		if err != nil {
			return err
		}
		if t == nil {
			return errTaskNotFound
		}
		if t.Status != StatusCompleted {
			return errInvalidState
		}
		highest, err := highestCompleted(ctx, tx, saleID)
		if err != nil {
			return err
		}
		if number != highest {
			return errNotLastCompleted
		}
		busy, err := anyInProgress(ctx, tx, saleID)
		if err != nil {
			return err
		}
		if busy {
			return errAlreadyInProgress
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET status = 'in_progress', completed_at = NULL,
			completion_phase = NULL, updated_by = $2, updated_at = $3, row_version = row_version + 1
			WHERE id = $1`, t.ID, actor.ID, now); err != nil {
			return err
		}
		if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionReopened, reason, map[string]any{"taskNumber": number}, now); err != nil {
			return err
		}
		if sale.State == SaleCompleted {
			if err := s.sales.MarkReactivated(ctx, tx, saleID, actor.ID, now); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, SaleID: &saleID, EntityType: audit.EntitySaleRegister, EntityID: saleID,
				Action: audit.ActionReopened, Reason: reason, Changes: map[string]any{"trigger": "task_13_reopened"},
			}, now); err != nil {
				return err
			}
		}
		sale, err = s.refreshed(ctx, tx, saleID)
		if err != nil {
			return err
		}
		result, err = s.detail(ctx, tx, sale, number)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// SaveData saves a draft of the data of the task in progress. The caller sends the task's version.
// A completed task is edited through a different, audited path that ships in 1G-b.
func (s *Service) SaveData(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int, in SaveInput) (*SaveResult, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	if in.Version == nil {
		return nil, apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	}
	now := s.now()
	var result *SaveResult
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, t, err := s.lockedTask(ctx, tx, actor, saleID, number)
		if err != nil {
			return err
		}
		if t.Version != *in.Version {
			return errVersionConflict
		}
		if t.Status != StatusInProgress {
			return errNotInProgress
		}
		d, err := kindFor(number).save(ctx, tx, sale, in.Data, actor.ID, now, s.sales)
		if err != nil {
			return err
		}
		if problem := d.result(); problem != nil {
			return problem
		}
		if len(d.changes) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE task_instances SET updated_by = $2, updated_at = $3, row_version = row_version + 1
				WHERE id = $1`, t.ID, actor.ID, now); err != nil {
				return err
			}
			if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionUpdated, "", d.changes, now); err != nil {
				return err
			}
		}
		sale, err = s.refreshed(ctx, tx, saleID)
		if err != nil {
			return err
		}
		detail, err := s.detail(ctx, tx, sale, number)
		if err != nil {
			return err
		}
		warnings := d.warnings
		if warnings == nil {
			warnings = []Warning{}
		}
		result = &SaveResult{Task: detail, Warnings: warnings}
		return nil
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// SaveNotes replaces the internal notes of a task. Notes are free text with no version: the last save
// wins, and saving them never makes a form in progress stale. Advisors write notes only while the task is
// in progress.
func (s *Service) SaveNotes(ctx context.Context, actor auth.Actor, saleID uuid.UUID, number int, raw string) (*Detail, error) {
	if err := checkNumber(number); err != nil {
		return nil, err
	}
	notes := strings.TrimSpace(raw)
	if utf8.RuneCountInString(notes) > 2000 {
		return nil, apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").
			WithFields(apperr.FieldError{Field: "notes", Code: "TOO_LONG", Message: "Las notas no pueden superar los 2000 caracteres."})
	}
	var value *string
	if notes != "" {
		value = &notes
	}
	now := s.now()
	var result *Detail
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, t, err := s.lockedTask(ctx, tx, actor, saleID, number)
		if err != nil {
			return err
		}
		if !isManagement(actor) && t.Status != StatusInProgress {
			return errNotInProgress
		}
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET notes = $2, updated_by = $3, updated_at = $4 WHERE id = $1`,
			t.ID, value, actor.ID, now); err != nil {
			return err
		}
		if err := s.recordTask(ctx, tx, actor, saleID, t.ID, audit.ActionUpdated, "", map[string]any{"notes": "changed"}, now); err != nil {
			return err
		}
		result, err = s.detail(ctx, tx, sale, number)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// CreateForSale adds the 13 pending tasks of a new sale and the three one-to-one data rows the commit-time
// trigger requires. The sales module calls it inside the transaction that creates the sale.
func CreateForSale(ctx context.Context, q Querier, saleID, actor uuid.UUID, now time.Time) error {
	for number := 1; number <= LastTask; number++ {
		if _, err := q.Exec(ctx, `INSERT INTO task_instances (id, sale_id, task_number, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)`, ids.New(), saleID, number, actor, now); err != nil {
			return err
		}
	}
	for _, table := range []string{"sale_financials", "sale_due_diligence", "sale_fulfillment"} {
		if _, err := q.Exec(ctx, `INSERT INTO `+table+` (sale_id) VALUES ($1)`, saleID); err != nil {
			return err
		}
	}
	return nil
}

// ShiftDueDate moves the due date of the task in progress forward. A cancelled sale that is reopened
// resumes with the time it had left, so its due date moves by the time spent cancelled (S5).
func ShiftDueDate(ctx context.Context, q Querier, saleID uuid.UUID, by time.Duration) error {
	_, err := q.Exec(ctx, `UPDATE task_instances SET due_date = due_date + make_interval(secs => $2::double precision)
		WHERE sale_id = $1 AND status = 'in_progress' AND due_date IS NOT NULL`, saleID, by.Seconds())
	return err
}
