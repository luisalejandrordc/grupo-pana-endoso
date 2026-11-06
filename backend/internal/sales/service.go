package sales

import (
	"context"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/tasks"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	errNotFound        = apperr.NotFound("SALE_NOT_FOUND", "La venta no existe.")
	errForbidden       = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
	errVersionConflict = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó este registro. Recargue e intente nuevamente.")
	errNotActive       = apperr.Conflict("SALE_NOT_ACTIVE", "La venta no está activa.")
	errNotCancelled    = apperr.Conflict("SALE_NOT_CANCELLED", "La venta no está cancelada.")
)

// Notifier is told about sale events that will produce notifications. Phase 2D replaces the no-op with the
// outbox writer; until then nothing is queued.
type Notifier interface {
	SaleCreated(ctx context.Context, q tasks.Querier, saleID uuid.UUID, now time.Time) error
	AdvisorChanged(ctx context.Context, q tasks.Querier, saleID uuid.UUID, previous, next uuid.UUID, now time.Time) error
}

// NoopNotifier ignores every event.
type NoopNotifier struct{}

// SaleCreated does nothing.
func (NoopNotifier) SaleCreated(context.Context, tasks.Querier, uuid.UUID, time.Time) error {
	return nil
}

// AdvisorChanged does nothing.
func (NoopNotifier) AdvisorChanged(context.Context, tasks.Querier, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}

// Deps are the service's dependencies.
type Deps struct {
	DB       auth.Database
	Notifier Notifier // nil means NoopNotifier
	Logger   *slog.Logger
	Now      func() time.Time // nil means time.Now
}

// Service implements the sale use cases.
type Service struct {
	db       auth.Database
	notifier Notifier
	logger   *slog.Logger
	now      func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, notifier: d.Notifier, logger: d.Logger, now: d.Now}
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

func oneOf(value string, allowed []string) bool {
	for _, a := range allowed {
		if a == value {
			return true
		}
	}
	return false
}

// visible applies the actor's scope: an Advisor only reaches the sales assigned to them.
func visible(actor auth.Actor, s *Sale) bool {
	return s != nil && (actor.Role != auth.RoleSalesAdvisor || s.AdvisorID == actor.ID)
}

// load finds a sale the actor may see, optionally locking it for the rest of the transaction.
func (s *Service) load(ctx context.Context, q tasks.Querier, actor auth.Actor, id uuid.UUID, forUpdate bool) (*Sale, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	sale, err := findSale(ctx, q, id, forUpdate)
	if err != nil {
		return nil, err
	}
	if !visible(actor, sale) {
		return nil, errNotFound
	}
	return sale, nil
}

func (s *Service) detail(ctx context.Context, q tasks.Querier, id uuid.UUID) (*Detail, error) {
	sale, err := findSale(ctx, q, id, false)
	if err != nil {
		return nil, err
	}
	if sale == nil {
		return nil, errNotFound
	}
	milestones, err := milestonesOf(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return &Detail{Sale: *sale, Milestones: milestones}, nil
}

// List returns one page of the workspace list.
func (s *Service) List(ctx context.Context, actor auth.Actor, f ListFilter) (*ListResult, error) {
	if !isStaff(actor) {
		return nil, errForbidden
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
	}
	items, total, err := listSales(ctx, s.db, f, scope)
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Page: f.Page, PageSize: f.PageSize, Total: total}, nil
}

// Get returns a sale with its milestones.
func (s *Service) Get(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Detail, error) {
	if _, err := s.load(ctx, s.db, actor, id, false); err != nil {
		return nil, err
	}
	return s.detail(ctx, s.db, id)
}

func fieldProblem(code, field, message string) *apperr.Error {
	return apperr.Unprocessable(code, message).WithFields(apperr.FieldError{Field: field, Code: code, Message: message})
}

// Create registers a sale (S1): it snapshots the customer and the vehicle, assigns the advisor and creates the
// 13 pending tasks, all in one transaction.
func (s *Service) Create(ctx context.Context, actor auth.Actor, in CreateInput) (*Detail, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	var problems []apperr.FieldError
	missing := func(field, label string, absent bool) {
		if absent {
			problems = append(problems, apperr.FieldError{Field: field, Code: "REQUIRED", Message: label + " es obligatorio."})
		}
	}
	missing("customerId", "El cliente", in.CustomerID == uuid.Nil)
	missing("versionId", "La versión", in.VersionID == uuid.Nil)
	missing("colorId", "El color", in.ColorID == uuid.Nil)
	if !oneOf(in.Temperature, temperatures) {
		problems = append(problems, apperr.FieldError{Field: "temperature", Code: "INVALID_VALUE", Message: "La temperatura no es válida."})
	}
	notes := strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(notes) > 2000 {
		problems = append(problems, apperr.FieldError{Field: "notes", Code: "TOO_LONG", Message: "Las notas no pueden superar los 2000 caracteres."})
	}
	advisor := actor.ID
	if actor.Role == auth.RoleSalesAdvisor {
		if in.AssignedAdvisorID != nil && *in.AssignedAdvisorID != actor.ID {
			return nil, apperr.Forbidden("ADVISOR_MISMATCH", "Un asesor solo puede crear ventas asignadas a sí mismo.")
		}
	} else if in.AssignedAdvisorID == nil || *in.AssignedAdvisorID == uuid.Nil {
		missing("assignedAdvisorId", "El asesor asignado", true)
	} else {
		advisor = *in.AssignedAdvisorID
	}
	if len(problems) > 0 {
		return nil, apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").WithFields(problems...)
	}
	var notesValue *string
	if notes != "" {
		notesValue = &notes
	}

	now := s.now()
	id := ids.New()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		customer, err := readCustomerSnapshot(ctx, tx, in.CustomerID)
		if err != nil {
			return err
		}
		if customer == nil {
			return fieldProblem("CUSTOMER_NOT_FOUND", "customerId", "El cliente no existe.")
		}
		if !customer.Active {
			return fieldProblem("CUSTOMER_INACTIVE", "customerId", "El cliente está inactivo y no puede recibir nuevas ventas.")
		}
		vehicle, err := readVehicleSnapshot(ctx, tx, in.VersionID, in.ColorID)
		if err != nil {
			return err
		}
		switch {
		case vehicle == nil:
			return fieldProblem("VEHICLE_NOT_SELECTABLE", "versionId", "La versión o el color indicado no existe.")
		case !vehicle.ColorBelongsToModel:
			return fieldProblem("VEHICLE_COLOR_MISMATCH", "colorId", "El color no corresponde al modelo de la versión.")
		case !vehicle.VersionSelectable:
			return fieldProblem("VEHICLE_NOT_SELECTABLE", "versionId", "La versión no está disponible para nuevas ventas.")
		case !vehicle.ColorSelectable:
			return fieldProblem("VEHICLE_NOT_SELECTABLE", "colorId", "El color no está disponible para nuevas ventas.")
		}
		if actor.Role != auth.RoleSalesAdvisor {
			active, err := advisorIsActive(ctx, tx, advisor)
			if err != nil {
				return err
			}
			if !active {
				return fieldProblem("INVALID_ADVISOR", "assignedAdvisorId", "El asesor indicado no existe o está inactivo.")
			}
		}
		if err := insertSale(ctx, tx, id, in, customer, vehicle, advisor, actor.ID, notesValue, now); err != nil {
			return err
		}
		if err := tasks.CreateForSale(ctx, tx, id, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySaleRegister, EntityID: id, Action: audit.ActionCreated,
			Changes: map[string]any{
				"customerId": in.CustomerID, "versionId": in.VersionID, "colorId": in.ColorID,
				"temperature": in.Temperature, "advisorId": advisor,
			},
		}, now); err != nil {
			return err
		}
		return s.notifier.SaleCreated(ctx, tx, id, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, id)
}

// UpdateNotes replaces the notes of a sale. Notes are the only editable field of the sale itself.
func (s *Service) UpdateNotes(ctx context.Context, actor auth.Actor, id uuid.UUID, in UpdateInput) (*Detail, error) {
	if in.Version == nil {
		return nil, apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	}
	var value *string
	if in.Notes != nil {
		notes := strings.TrimSpace(*in.Notes)
		if utf8.RuneCountInString(notes) > 2000 {
			return nil, fieldProblem("VALIDATION_FAILED", "notes", "Las notas no pueden superar los 2000 caracteres.")
		}
		if notes != "" {
			value = &notes
		}
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.load(ctx, tx, actor, id, true)
		if err != nil {
			return err
		}
		if sale.State == StateCancelled {
			return errNotActive
		}
		ok, err := setNotes(ctx, tx, id, *in.Version, value, actor.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			return errVersionConflict
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySaleRegister, EntityID: id,
			Action: audit.ActionUpdated, Changes: map[string]any{"notes": "changed"},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, id)
}

// Reassign gives an active sale to another active advisor (S6). The previous advisor loses access with the
// commit, because scope is read from the sale itself.
func (s *Service) Reassign(ctx context.Context, actor auth.Actor, id uuid.UUID, in ReassignInput) (*Detail, error) {
	if !isManagement(actor) {
		return nil, errForbidden
	}
	if in.AdvisorID == uuid.Nil {
		return nil, fieldProblem("VALIDATION_FAILED", "advisorId", "El asesor es obligatorio.")
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.load(ctx, tx, actor, id, true)
		if err != nil {
			return err
		}
		if sale.State != StateActive {
			return errNotActive
		}
		if sale.AdvisorID == in.AdvisorID {
			return fieldProblem("SAME_ADVISOR", "advisorId", "La venta ya está asignada a ese asesor.")
		}
		active, err := advisorIsActive(ctx, tx, in.AdvisorID)
		if err != nil {
			return err
		}
		if !active {
			return fieldProblem("INVALID_ADVISOR", "advisorId", "El asesor indicado no existe o está inactivo.")
		}
		if err := setAdvisor(ctx, tx, id, in.AdvisorID, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySaleRegister, EntityID: id, Action: audit.ActionReassigned,
			Changes: map[string]any{"from": sale.AdvisorID, "to": in.AdvisorID},
		}, now); err != nil {
			return err
		}
		return s.notifier.AdvisorChanged(ctx, tx, id, sale.AdvisorID, in.AdvisorID, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, id)
}

// canCancel applies the cancellation table of the spec: an Advisor may cancel their own sale while Tasks 1
// to 3 are current, a Manager through Task 6, and an Admin at any point.
func canCancel(actor auth.Actor, sale *Sale, current int) bool {
	switch actor.Role {
	case auth.RoleSystemAdmin:
		return true
	case auth.RoleSalesManager:
		return current <= 6
	case auth.RoleSalesAdvisor:
		return sale.AdvisorID == actor.ID && current <= 3
	}
	return false
}

// Cancel freezes an active sale (S4).
func (s *Service) Cancel(ctx context.Context, actor auth.Actor, id uuid.UUID, in CancelInput) (*Detail, error) {
	if !isStaff(actor) {
		return nil, errForbidden
	}
	text := strings.TrimSpace(in.Text)
	switch {
	case !oneOf(in.Category, cancellationCategories):
		return nil, fieldProblem("VALIDATION_FAILED", "category", "La categoría de cancelación no es válida.")
	case in.Category == "other" && text == "":
		return nil, fieldProblem("VALIDATION_FAILED", "text", "Debe describir el motivo de la cancelación.")
	case utf8.RuneCountInString(text) > 500:
		return nil, fieldProblem("VALIDATION_FAILED", "text", "El motivo no puede superar los 500 caracteres.")
	}
	var textValue *string
	reason := in.Category
	if text != "" {
		textValue, reason = &text, text
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.load(ctx, tx, actor, id, true)
		if err != nil {
			return err
		}
		if sale.State != StateActive || sale.CurrentTaskNumber == nil {
			return errNotActive
		}
		current := *sale.CurrentTaskNumber
		if !canCancel(actor, sale, current) {
			return apperr.Forbidden("CANCEL_NOT_ALLOWED", "Su rol no puede cancelar la venta en esta etapa.")
		}
		if err := markCancelled(ctx, tx, id, current, in.Category, textValue, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySaleRegister, EntityID: id, Action: audit.ActionCancelled,
			Reason: reason, Changes: map[string]any{"category": in.Category, "taskNumber": current},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, id)
}

// Reopen brings a cancelled sale back to active (S5). A Manager may do it when the sale was cancelled while
// Tasks 1 to 6 were current; later cancellations need an Admin. The due date of the task in progress moves
// forward by the time the sale spent cancelled, so the task resumes with the time it had left.
func (s *Service) Reopen(ctx context.Context, actor auth.Actor, id uuid.UUID, in ReasonInput) (*Detail, error) {
	if !isManagement(actor) {
		return nil, errForbidden
	}
	reason := strings.TrimSpace(in.Reason)
	switch {
	case reason == "":
		return nil, fieldProblem("REASON_REQUIRED", "reason", "El motivo es obligatorio.")
	case utf8.RuneCountInString(reason) > 500:
		return nil, fieldProblem("REASON_TOO_LONG", "reason", "El motivo no puede superar los 500 caracteres.")
	}
	now := s.now()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		sale, err := s.load(ctx, tx, actor, id, true)
		if err != nil {
			return err
		}
		if sale.State != StateCancelled || sale.CancelledAt == nil || sale.CancelledAtTask == nil {
			return errNotCancelled
		}
		if *sale.CancelledAtTask >= 7 && actor.Role != auth.RoleSystemAdmin {
			return apperr.Forbidden("REOPEN_NOT_ALLOWED", "Solo un administrador puede reabrir una venta cancelada después de la Tarea 6.")
		}
		away := now.Sub(*sale.CancelledAt)
		if err := markReopened(ctx, tx, id, actor.ID, now); err != nil {
			return err
		}
		if err := tasks.ShiftDueDate(ctx, tx, id, away); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, SaleID: &id, EntityType: audit.EntitySaleRegister, EntityID: id, Action: audit.ActionReopened,
			Reason: reason, Changes: map[string]any{"cancelledAtTaskNumber": *sale.CancelledAtTask, "shiftedSeconds": int64(away.Seconds())},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return s.detail(ctx, s.db, id)
}

// History returns one page of the sale's audit timeline, newest first, to anyone who can see the sale.
func (s *Service) History(ctx context.Context, actor auth.Actor, id uuid.UUID, page, pageSize int) (*HistoryResult, error) {
	if _, err := s.load(ctx, s.db, actor, id, false); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := listHistory(ctx, s.db, id, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &HistoryResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}
