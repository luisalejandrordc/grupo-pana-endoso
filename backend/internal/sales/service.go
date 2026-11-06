package sales

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/outbox"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/storagecleanup"
	"endosoflow/internal/tasks"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	errForbidden       = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
	errNotFound        = apperr.NotFound("SALE_NOT_FOUND", "La venta no existe.")
	errVersionRequired = apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	errVersionConflict = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó esta venta. Recargue e intente nuevamente.")
	errNotActive       = apperr.Conflict("SALE_NOT_ACTIVE", "La venta no está activa.")
	errNotCancelled    = apperr.Conflict("SALE_NOT_CANCELLED", "La venta no está cancelada.")
	errReasonRequired  = apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo.").
				WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio."})
)

// Deps are the service's dependencies.
type Deps struct {
	DB     auth.Database
	Store  storage.Store
	Logger *slog.Logger
	Now    func() time.Time // nil means time.Now
}

// Service implements the sale use cases.
type Service struct {
	db     auth.Database
	store  storage.Store
	logger *slog.Logger
	now    func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, store: d.Store, logger: d.Logger, now: d.Now}
	if s.now == nil {
		s.now = time.Now
	}
	if s.store == nil {
		s.store = storage.Unconfigured()
	}
	return s
}

func isStaff(a auth.Actor) bool {
	return a.Role == auth.RoleSystemAdmin || a.Role == auth.RoleSalesManager || a.Role == auth.RoleSalesAdvisor
}

func requireStaff(a auth.Actor) error {
	if !isStaff(a) {
		return errForbidden
	}
	return nil
}

func validationError(field, code, message string) error {
	return apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").
		WithFields(apperr.FieldError{Field: field, Code: code, Message: message})
}

// visible loads a sale for an actor: an Advisor only sees their own, and anything else does not exist for them.
func (s *Service) lockVisible(ctx context.Context, q querier, actor auth.Actor, id uuid.UUID) (*tasks.SaleRow, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	sale, err := tasks.LockSale(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if sale == nil || (actor.Role == auth.RoleSalesAdvisor && sale.AdvisorID != actor.ID) {
		return nil, errNotFound
	}
	return sale, nil
}

// --- reading ---

// Get returns the full sale detail the actor is allowed to see.
func (s *Service) Get(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Sale, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	return s.detail(ctx, s.db, actor, id)
}

func (s *Service) detail(ctx context.Context, q querier, actor auth.Actor, id uuid.UUID) (*Sale, error) {
	row, err := loadSaleRow(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if row == nil || (actor.Role == auth.RoleSalesAdvisor && row.advisorID != actor.ID) {
		return nil, errNotFound
	}
	sale := row.Sale
	if row.imageKey != nil {
		url := s.store.PublicURL(*row.imageKey)
		sale.Vehicle.ImageURL = &url
	}
	summaries, err := tasks.Summaries(ctx, q, id)
	if err != nil {
		return nil, err
	}
	sale.Tasks = summaries
	statuses := make([]tasks.Status, len(summaries))
	for i, t := range summaries {
		statuses[i] = t.Status
	}
	defs, err := loadMilestoneDefs(ctx, q)
	if err != nil {
		return nil, err
	}
	states := tasks.DeriveMilestones(statuses)
	sale.Milestones = make([]Milestone, len(defs))
	for i, d := range defs {
		sale.Milestones[i] = Milestone{Number: d.number, Name: d.name, Description: d.description, State: states[d.number-1]}
	}
	current := tasks.CurrentTask(statuses)
	if current > 0 {
		t := summaries[current-1]
		sale.CurrentTask = &CurrentTask{
			Number: t.Number, Name: t.Name, Status: t.Status, DueDate: t.DueDate, Milestone: t.MilestoneNumber,
			IsOverdue: t.Status == tasks.StatusInProgress && t.DueDate != nil && t.DueDate.Before(s.now()),
		}
	}
	sale.Permissions, err = s.permissions(ctx, q, actor, row, current)
	if err != nil {
		return nil, err
	}
	return &sale, nil
}

func (s *Service) permissions(ctx context.Context, q querier, actor auth.Actor, row *saleRow, current int) (Permissions, error) {
	ownSale := row.advisorID == actor.ID
	p := Permissions{
		CanEditNotes: tasks.CanActOnTask(actor.Role, ownSale),
		CanCancel:    row.State == "active" && tasks.CanCancel(actor.Role, ownSale, current),
		CanReassign:  row.State == "active" && tasks.IsManagement(actor.Role),
	}
	if row.State == "cancelled" && row.cancelledAtNum != nil {
		p.CanReopen = tasks.CanReopenSale(actor.Role, *row.cancelledAtNum)
	}
	if actor.Role == auth.RoleSystemAdmin {
		delivered, err := everDelivered(ctx, q, row.ID)
		if err != nil {
			return p, err
		}
		p.CanDelete = !delivered
	}
	return p, nil
}

// everDelivered is true if task 13 was ever completed, even if it was reopened later.
func everDelivered(ctx context.Context, q querier, saleID uuid.UUID) (bool, error) {
	var delivered bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_logs l JOIN task_instances t ON t.id = l.entity_id
		WHERE l.sale_id = $1 AND l.entity_type = 'task_instance' AND l.action = 'COMPLETED' AND t.task_number = 13)`, saleID).Scan(&delivered)
	return delivered, err
}

// List returns one page of the workspace. An Advisor only ever gets their own sales, in the list and in the
// count.
func (s *Service) List(ctx context.Context, actor auth.Actor, f ListFilter) (*List, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	var scope *uuid.UUID
	if actor.Role == auth.RoleSalesAdvisor {
		scope = &actor.ID
		f.AdvisorID = nil
	}
	rows, total, err := listSales(ctx, s.db, f, scope, s.now())
	if err != nil {
		return nil, err
	}
	items := make([]ListItem, len(rows))
	for i, r := range rows {
		items[i] = r.ListItem
		items[i].CurrentTask = r.CurrentTask
		if r.imageKey != nil {
			url := s.store.PublicURL(*r.imageKey)
			items[i].ImageURL = &url
		}
	}
	return &List{Items: items, Page: f.Page, PageSize: f.PageSize, Total: total}, nil
}

// History returns the sale's timeline, newest first.
func (s *Service) History(ctx context.Context, actor auth.Actor, id uuid.UUID, page, pageSize int) (*History, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	row, err := loadSaleRow(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if row == nil || (actor.Role == auth.RoleSalesAdvisor && row.advisorID != actor.ID) {
		return nil, errNotFound
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := loadHistory(ctx, s.db, id, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &History{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// --- creating ---

// Create starts a sale: it records the customer and vehicle as they are now, creates the 13 pending tasks and
// assigns the advisor. An Advisor creates for themselves; a Manager or Admin chooses an active Advisor.
func (s *Service) Create(ctx context.Context, actor auth.Actor, in CreateInput) (*Sale, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	switch in.Temperature {
	case "hot", "warm", "cold":
	default:
		return nil, validationError("temperature", "INVALID_VALUE", "La temperatura debe ser hot, warm o cold.")
	}
	notes := strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(notes) > 2000 {
		return nil, validationError("notes", "TOO_LONG", "Las notas son demasiado largas (máximo 2000 caracteres).")
	}
	var advisorID uuid.UUID
	if actor.Role == auth.RoleSalesAdvisor {
		if in.AdvisorID != nil && *in.AdvisorID != actor.ID {
			return nil, validationError("advisorId", "NOT_ALLOWED", "Un asesor solo puede crear ventas para sí mismo.")
		}
		advisorID = actor.ID
	} else {
		if in.AdvisorID == nil {
			return nil, validationError("advisorId", "REQUIRED", "Elige el asesor que llevará la venta.")
		}
		advisorID = *in.AdvisorID
	}

	now := s.now()
	id := ids.New()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		advisor, err := loadAdvisor(ctx, tx, advisorID)
		if err != nil {
			return err
		}
		if advisor == nil || advisor.Role != string(auth.RoleSalesAdvisor) || !advisor.Active {
			return validationError("advisorId", "INVALID_ADVISOR", "El asesor debe ser un asesor de ventas activo.")
		}
		customer, err := loadCustomer(ctx, tx, in.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return apperr.NotFound("CUSTOMER_NOT_FOUND", "El cliente no existe.")
		}
		if !customer.Active {
			return apperr.Conflict("CUSTOMER_INACTIVE", "El cliente está inactivo y no puede recibir nuevas ventas.")
		}
		vehicle, err := loadVehicle(ctx, tx, in.VersionID, in.ColorID)
		if err != nil {
			return err
		}
		switch {
		case !vehicle.VersionFound:
			return validationError("versionId", "NOT_FOUND", "La versión no existe.")
		case !vehicle.Selectable:
			return validationError("versionId", "NOT_AVAILABLE", "La versión no está disponible: está archivada o su modelo está archivado.")
		case !vehicle.ColorFound:
			return validationError("colorId", "NOT_FOUND", "El color no existe.")
		case vehicle.ColorModelID != vehicle.ModelID:
			return validationError("colorId", "COLOR_MODEL_MISMATCH", "El color elegido no pertenece al modelo de la versión.")
		case !vehicle.ColorOK:
			return validationError("colorId", "NOT_AVAILABLE", "El color no está disponible: está archivado o no tiene imagen principal.")
		}

		var notesValue any
		if notes != "" {
			notesValue = notes
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sale_registers
			(id, customer_id, customer_legal_name_snapshot, customer_doc_type_snapshot, customer_doc_number_snapshot, customer_type_snapshot,
			 vehicle_model_id, vehicle_version_id, vehicle_color_id, vehicle_model_name_snapshot, vehicle_version_name_snapshot,
			 vehicle_color_name_snapshot, vehicle_reference_price_snapshot, sale_temperature, assigned_advisor_id, sale_notes,
			 created_by, created_at, updated_by, updated_at)
			VALUES ($1, $2, $3, $4::text::customer_document_type, $5, $6::text::customer_type, $7, $8, $9, $10, $11, $12, $13::numeric,
			        $14::text::sale_temperature, $15, $16, $17, $18, $17, $18)`,
			id, customer.ID, customer.Name, customer.DocType, customer.DocNumber, customer.CustomerType,
			vehicle.ModelID, in.VersionID, in.ColorID, vehicle.ModelName, vehicle.VersionName, vehicle.ColorName, vehicle.Price,
			in.Temperature, advisorID, notesValue, actor.ID, now); err != nil {
			return err
		}
		if err := tasks.CreateInitial(ctx, tx, id, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionCreated, Changes: map[string]any{
				"customerId": customer.ID, "versionId": in.VersionID, "colorId": in.ColorID,
				"temperature": in.Temperature, "advisorId": advisorID,
			},
		}, now); err != nil {
			return err
		}
		contact, err := tasks.LoadCustomerContact(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := outbox.Enqueue(ctx, tx, outbox.Notification{
			Type: outbox.SaleCreated, DedupeKey: "sale_created:" + id.String(),
			SaleID: &id, CustomerID: &contact.ID, Email: contact.Email,
		}); err != nil {
			return err
		}
		// An advisor who creates the sale for themselves is not told about it; a manager's assignment is.
		if advisorID != actor.ID {
			return outbox.Enqueue(ctx, tx, outbox.Notification{
				Type: outbox.SaleAssigned, DedupeKey: "sale_assigned:" + id.String() + ":" + advisorID.String(),
				SaleID: &id, EmployeeID: &advisorID, Email: advisor.Email,
			})
		}
		return nil
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, actor, id)
}

// --- notes ---

// UpdateNotes changes the sale's internal notes, the only editable field of a sale besides its advisor.
func (s *Service) UpdateNotes(ctx context.Context, actor auth.Actor, id uuid.UUID, version *int, notes string) (*Sale, error) {
	if version == nil {
		return nil, errVersionRequired
	}
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > 2000 {
		return nil, validationError("notes", "TOO_LONG", "Las notas son demasiado largas (máximo 2000 caracteres).")
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.lockVisible(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if !tasks.CanActOnTask(actor.Role, sale.AdvisorID == actor.ID) {
			return errForbidden
		}
		var value any
		if notes != "" {
			value = notes
		}
		tag, err := tx.Exec(ctx, `UPDATE sale_registers SET sale_notes = $2, updated_by = $3, updated_at = $4, row_version = row_version + 1
			WHERE id = $1 AND row_version = $5`, id, value, actor.ID, now, *version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errVersionConflict
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionUpdated, Changes: map[string]any{"fields": []string{"notes"}},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, actor, id)
}

// --- reassigning ---

// Reassign hands the sale to another active Advisor (Manager, Admin). The customer, the new advisor and the
// previous advisor are notified.
func (s *Service) Reassign(ctx context.Context, actor auth.Actor, id uuid.UUID, advisorID uuid.UUID, reason string) (*Sale, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.lockVisible(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if !tasks.IsManagement(actor.Role) {
			return errForbidden
		}
		if sale.State != "active" {
			return errNotActive
		}
		if sale.AdvisorID == advisorID {
			return apperr.Conflict("SALE_ALREADY_ASSIGNED", "La venta ya está asignada a ese asesor.")
		}
		next, err := loadAdvisor(ctx, tx, advisorID)
		if err != nil {
			return err
		}
		if next == nil || next.Role != string(auth.RoleSalesAdvisor) || !next.Active {
			return validationError("advisorId", "INVALID_ADVISOR", "El asesor debe ser un asesor de ventas activo.")
		}
		previous, err := loadAdvisor(ctx, tx, sale.AdvisorID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sale_registers SET assigned_advisor_id = $2, updated_by = $3, updated_at = $4, row_version = row_version + 1
			WHERE id = $1`, id, advisorID, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionReassigned, Reason: strings.TrimSpace(reason),
			Changes: map[string]any{"from": sale.AdvisorID, "to": advisorID},
		}, now); err != nil {
			return err
		}
		contact, err := tasks.LoadCustomerContact(ctx, tx, id)
		if err != nil {
			return err
		}
		// Each change gets its own key, so reassigning back and forth notifies every time.
		stamp := ids.New().String()
		for _, n := range []outbox.Notification{
			{Type: outbox.CustomerAdvisorChanged, DedupeKey: "advisor_changed:" + id.String() + ":" + stamp, SaleID: &id, CustomerID: &contact.ID, Email: contact.Email},
			{Type: outbox.SaleAssigned, DedupeKey: "sale_assigned:" + id.String() + ":" + stamp, SaleID: &id, EmployeeID: &advisorID, Email: next.Email},
		} {
			if err := outbox.Enqueue(ctx, tx, n); err != nil {
				return err
			}
		}
		if previous != nil {
			return outbox.Enqueue(ctx, tx, outbox.Notification{
				Type: outbox.SaleUnassigned, DedupeKey: "sale_unassigned:" + id.String() + ":" + stamp,
				SaleID: &id, EmployeeID: &previous.ID, Email: previous.Email,
			})
		}
		return nil
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, actor, id)
}

// --- cancelling and reopening ---

var cancellationCategories = map[string]bool{
	"customer_withdrew": true, "payment_or_financing_failed": true, "unit_unavailable": true,
	"due_diligence_finding": true, "registration_error": true, "other": true,
}

// Cancel freezes an active sale. An Advisor may cancel their own sale while the current task is 1 to 3, a
// Manager while it is 1 to 6, and an Admin at any stage. The reason needs a category and a text.
func (s *Service) Cancel(ctx context.Context, actor auth.Actor, id uuid.UUID, category, text string) (*Sale, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if !cancellationCategories[category] {
		return nil, validationError("category", "INVALID_VALUE", "Elige una categoría de cancelación válida.")
	}
	if text == "" {
		return nil, validationError("reason", "REQUIRED", "Escribe el motivo de la cancelación.")
	}
	if utf8.RuneCountInString(text) > 1000 {
		return nil, validationError("reason", "TOO_LONG", "El motivo es demasiado largo (máximo 1000 caracteres).")
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.lockVisible(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if sale.State != "active" {
			return errNotActive
		}
		summaries, err := tasks.Summaries(ctx, tx, id)
		if err != nil {
			return err
		}
		statuses := make([]tasks.Status, len(summaries))
		for i, t := range summaries {
			statuses[i] = t.Status
		}
		current := tasks.CurrentTask(statuses)
		if !tasks.CanCancel(actor.Role, sale.AdvisorID == actor.ID, current) {
			return apperr.Forbidden("CANCEL_NOT_ALLOWED", "Tu rol no puede cancelar una venta en esta etapa. Pide a un gerente o administrador que lo haga.")
		}
		if _, err := tx.Exec(ctx, `UPDATE sale_registers SET overall_state = 'cancelled', cancelled_at = $2, cancelled_by = $3, cancelled_at_task_number = $4,
				cancellation_category = $5::text::cancellation_category, cancellation_text = $6, updated_by = $3, updated_at = $2, row_version = row_version + 1
			WHERE id = $1`, id, now, actor.ID, current, category, text); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionCancelled, Reason: text, Changes: map[string]any{"category": category, "atTaskNumber": current},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, actor, id)
}

// Reopen brings a cancelled sale back to active (Manager only if it was cancelled at task 6 or earlier, Admin
// always), with a written reason. The clock of the in-progress task is moved forward by the time the sale spent
// cancelled, so it resumes with the time it had left.
func (s *Service) Reopen(ctx context.Context, actor auth.Actor, id uuid.UUID, reason string) (*Sale, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errReasonRequired
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.lockVisible(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if sale.State != "cancelled" || sale.CancelledAtTask == nil {
			return errNotCancelled
		}
		if !tasks.CanReopenSale(actor.Role, *sale.CancelledAtTask) {
			return apperr.Forbidden("REOPEN_NOT_ALLOWED", "Tu rol no puede reabrir esta venta. Una venta cancelada después de la tarea 6 solo la reabre un administrador.")
		}
		var cancelledAt time.Time
		if err := tx.QueryRow(ctx, `SELECT cancelled_at FROM sale_registers WHERE id = $1`, id).Scan(&cancelledAt); err != nil {
			return err
		}
		shift := now.Sub(cancelledAt).Seconds()
		if _, err := tx.Exec(ctx, `UPDATE task_instances SET started_at = started_at + make_interval(secs => $2),
				due_date = due_date + make_interval(secs => $2), updated_by = $3, updated_at = $4, row_version = row_version + 1
			WHERE sale_id = $1 AND status = 'in_progress'`, id, shift, actor.ID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sale_registers SET overall_state = 'active', cancelled_at = NULL, cancelled_by = NULL, cancelled_at_task_number = NULL,
				cancellation_category = NULL, cancellation_text = NULL, updated_by = $2, updated_at = $3, row_version = row_version + 1
			WHERE id = $1`, id, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionReopened, Reason: reason, Changes: map[string]any{"from": "cancelled", "to": "active", "shiftedSeconds": int64(shift)},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, actor, id)
}

// --- deleting ---

// Delete permanently removes a sale that never reached delivery (Admin only), with a reason and the typed sale
// code as confirmation. Its files leave storage after the commit; the audit rows survive.
func (s *Service) Delete(ctx context.Context, actor auth.Actor, id uuid.UUID, reason, confirm string) error {
	if !isStaff(actor) || actor.Role != auth.RoleSystemAdmin {
		if isStaff(actor) {
			if _, err := s.Get(ctx, actor, id); err != nil {
				return err
			}
		}
		return errForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errReasonRequired
	}
	now := s.now()
	var pending []storagecleanup.Pending
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		pending = nil
		if _, err := s.lockVisible(ctx, tx, actor, id); err != nil {
			return err
		}
		row, err := loadSaleRow(ctx, tx, id)
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(confirm), row.Code) {
			return validationError("confirm", "CONFIRMATION_MISMATCH", "Escribe el código de la venta ("+row.Code+") para confirmar.")
		}
		delivered, err := everDelivered(ctx, tx, id)
		if err != nil {
			return err
		}
		if delivered {
			return apperr.Conflict("SALE_ALREADY_DELIVERED", "Una venta que llegó a la entrega no puede eliminarse.")
		}
		keys, err := tx.Query(ctx, `SELECT d.storage_key FROM documents d JOIN task_instances t ON t.id = d.task_instance_id WHERE t.sale_id = $1`, id)
		if err != nil {
			return err
		}
		var files []string
		for keys.Next() {
			var k string
			if err := keys.Scan(&k); err != nil {
				keys.Close()
				return err
			}
			files = append(files, k)
		}
		keys.Close()
		if err := keys.Err(); err != nil {
			return err
		}
		for _, k := range files {
			p, err := storagecleanup.Enqueue(ctx, tx, k, storagecleanup.SaleDeleted)
			if err != nil {
				return err
			}
			pending = append(pending, p)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sale_registers WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySale, EntityID: id,
			Action: audit.ActionDeleted, Reason: reason,
			Changes: map[string]any{"code": row.Code, "customerName": row.Customer.Name, "state": row.State, "files": len(files)},
		}, now)
	})
	if err != nil {
		return apperr.FromError(err)
	}
	storagecleanup.Process(ctx, s.db, s.store, s.logger, pending...)
	return nil
}
