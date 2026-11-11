package staff

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	errNotFound        = apperr.NotFound("EMPLOYEE_NOT_FOUND", "El empleado no existe.")
	errVersionConflict = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó este registro. Recargue e intente nuevamente.")
	errForbidden       = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
)

// ActivationIssuer creates and emails an activation link (implemented by the auth service).
type ActivationIssuer interface {
	IssueActivation(ctx context.Context, employeeID uuid.UUID, createdBy *uuid.UUID) (string, error)
}

// AccessManager revokes or removes an employee's sessions and links (implemented by the auth service).
type AccessManager interface {
	RevokeAccess(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID, reason string, now time.Time) error
	PurgeTokens(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID) error
}

// Deps are the service's dependencies.
type Deps struct {
	DB         auth.Database
	Activation ActivationIssuer
	Access     AccessManager
	Avatars    storage.Store
	Logger     *slog.Logger
	Now        func() time.Time // nil means time.Now
}

// Service implements the staff use cases.
type Service struct {
	db         auth.Database
	activation ActivationIssuer
	access     AccessManager
	avatars    storage.Store
	logger     *slog.Logger
	now        func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	avatars := d.Avatars
	if avatars == nil {
		avatars = storage.Unconfigured()
	}
	return &Service{db: d.DB, activation: d.Activation, access: d.Access, avatars: avatars, logger: d.Logger, now: now}
}

func requireAdmin(actor auth.Actor) error {
	if actor.Role != auth.RoleSystemAdmin {
		return errForbidden
	}
	return nil
}

func stringOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// List returns one page of employees. Admin only.
func (s *Service) List(ctx context.Context, actor auth.Actor, f ListFilter) (*ListResult, error) {
	if err := requireAdmin(actor); err != nil {
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
	items, total, err := listEmployees(ctx, s.db, f)
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Page: f.Page, PageSize: f.PageSize, Total: total}, nil
}

// Get returns one employee. Admin only.
func (s *Service) Get(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Employee, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	employee, err := findEmployee(ctx, s.db, id, false)
	if err != nil {
		return nil, err
	}
	if employee == nil {
		return nil, errNotFound
	}
	return employee, nil
}

// Advisors lists the active Sales Advisors for pickers. Admins and Managers may call it.
func (s *Service) Advisors(ctx context.Context, actor auth.Actor) ([]Advisor, error) {
	if actor.Role != auth.RoleSystemAdmin && actor.Role != auth.RoleSalesManager {
		return nil, errForbidden
	}
	return listAdvisors(ctx, s.db)
}

// Create adds an employee and sends the activation link. The account has no password until the person
// opens the link. If issuing the link fails the employee still exists and an Admin can resend it.
func (s *Service) Create(ctx context.Context, actor auth.Actor, in CreateInput) (*CreateResult, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	now := s.now()
	data, problem := validateData(rawData{
		DocumentType: in.DocumentType, DocumentNumber: in.DocumentNumber, FirstName: in.FirstName,
		LastName: in.LastName, NameSource: in.NameSource, Nationality: in.Nationality, Gender: in.Gender,
		Birthdate: in.Birthdate, HireDate: in.HireDate, PersonalPhone: stringOf(in.PersonalPhone),
		PersonalEmail: stringOf(in.PersonalEmail), BusinessPhone: in.BusinessPhone,
		BusinessEmail: in.BusinessEmail, Role: in.Role,
	}, now)
	if problem != nil {
		return nil, problem
	}

	id := ids.New()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := insertEmployee(ctx, tx, id, data, actor.ID, now); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee, EntityID: id, Action: audit.ActionCreated,
			Changes: map[string]any{
				"documentType": data.DocumentType, "documentNumber": data.DocumentNumber,
				"role": data.Role,
			},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}

	created, err := findEmployee(ctx, s.db, id, false)
	if err != nil {
		return nil, err
	}
	issued := true
	if _, err := s.activation.IssueActivation(ctx, id, &actor.ID); err != nil {
		issued = false
		s.logger.Error("employee created but the activation link could not be issued", "employee_id", id, "error", err)
	}
	return &CreateResult{Employee: *created, ActivationLinkIssued: issued}, nil
}

func mergeUpdate(current Employee, in UpdateInput) rawData {
	pick := func(newValue *string, old string) string {
		if newValue != nil {
			return *newValue
		}
		return old
	}
	return rawData{
		DocumentType:   pick(in.DocumentType, current.DocumentType),
		DocumentNumber: pick(in.DocumentNumber, current.DocumentNumber),
		FirstName:      pick(in.FirstName, current.FirstName),
		LastName:       pick(in.LastName, current.LastName),
		NameSource:     pick(in.NameSource, current.NameSource),
		Nationality:    pick(in.Nationality, current.Nationality),
		Gender:         pick(in.Gender, current.Gender),
		Birthdate:      pick(in.Birthdate, current.Birthdate),
		HireDate:       pick(in.HireDate, current.HireDate),
		PersonalPhone:  pick(in.PersonalPhone, stringOf(current.PersonalPhone)),
		PersonalEmail:  pick(in.PersonalEmail, stringOf(current.PersonalEmail)),
		BusinessPhone:  pick(in.BusinessPhone, current.BusinessPhone),
		BusinessEmail:  pick(in.BusinessEmail, current.BusinessEmail),
		Role:           pick(in.Role, current.Role),
	}
}

// diffEmployee summarizes what changed. Personal contact data is recorded as "changed" without its values.
func diffEmployee(old Employee, d employeeData) (changes map[string]any, roleChange map[string]any) {
	changes = map[string]any{}
	note := func(field, before, after string) {
		if before != after {
			changes[field] = map[string]string{"before": before, "after": after}
		}
	}
	note("documentType", old.DocumentType, d.DocumentType)
	note("documentNumber", old.DocumentNumber, d.DocumentNumber)
	note("firstName", old.FirstName, d.FirstName)
	note("lastName", old.LastName, d.LastName)
	note("nameSource", old.NameSource, d.NameSource)
	note("nationality", old.Nationality, d.Nationality)
	note("gender", old.Gender, d.Gender)
	note("birthdate", old.Birthdate, d.Birthdate.Format("2006-01-02"))
	note("hireDate", old.HireDate, d.HireDate.Format("2006-01-02"))
	note("businessPhone", old.BusinessPhone, d.BusinessPhone)
	note("businessEmail", old.BusinessEmail, d.BusinessEmail)
	if stringOf(old.PersonalPhone) != stringOf(d.PersonalPhone) {
		changes["personalPhone"] = "changed"
	}
	if stringOf(old.PersonalEmail) != stringOf(d.PersonalEmail) {
		changes["personalEmail"] = "changed"
	}
	if old.Role != d.Role {
		roleChange = map[string]any{"role": map[string]string{"before": old.Role, "after": d.Role}}
	}
	return changes, roleChange
}

// Update changes an employee. The caller sends the version it read; a stale version is refused.
func (s *Service) Update(ctx context.Context, actor auth.Actor, id uuid.UUID, in UpdateInput) (*Employee, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if in.Version == nil {
		return nil, apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	}
	now := s.now()
	var result *Employee

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		current, err := findEmployee(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current == nil {
			return errNotFound
		}
		if current.Version != *in.Version {
			return errVersionConflict
		}
		data, problem := validateData(mergeUpdate(*current, in), now)
		if problem != nil {
			return problem
		}
		if current.Role == string(auth.RoleSalesAdvisor) && data.Role != current.Role {
			n, err := countActiveSales(ctx, tx, id)
			if err != nil {
				return err
			}
			if n > 0 {
				return apperr.Conflict("ADVISOR_HAS_ACTIVE_SALES",
					fmt.Sprintf("El asesor tiene %d venta(s) activa(s). Reasígnelas antes de cambiar su rol.", n))
			}
		}
		changed, err := updateEmployee(ctx, tx, id, *in.Version, data, actor.ID, now)
		if err != nil {
			return err
		}
		if !changed {
			return errVersionConflict
		}

		changes, roleChange := diffEmployee(*current, data)
		if len(changes) > 0 {
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, EntityType: audit.EntityEmployee,
				EntityID: id, Action: audit.ActionUpdated, Changes: changes,
			}, now); err != nil {
				return err
			}
		}
		if roleChange != nil {
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, EntityType: audit.EntityEmployee,
				EntityID: id, Action: audit.ActionRoleChanged, Changes: roleChange,
			}, now); err != nil {
				return err
			}
		}
		result, err = findEmployee(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

// Deactivate turns an account off and ends its sessions. History is kept. The last active Admin is
// protected by the database; here we also refuse self-deactivation and advisors with active sales.
func (s *Service) Deactivate(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Employee, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if id == actor.ID {
		return nil, apperr.Conflict("CANNOT_DEACTIVATE_SELF", "No puede desactivar su propia cuenta.")
	}
	now := s.now()
	var result *Employee

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		employee, err := findEmployee(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if employee == nil {
			return errNotFound
		}
		if !employee.IsActive {
			return apperr.Conflict("EMPLOYEE_ALREADY_INACTIVE", "El empleado ya está inactivo.")
		}
		if employee.Role == string(auth.RoleSalesAdvisor) {
			n, err := countActiveSales(ctx, tx, id)
			if err != nil {
				return err
			}
			if n > 0 {
				return apperr.Conflict("ADVISOR_HAS_ACTIVE_SALES",
					fmt.Sprintf("El asesor tiene %d venta(s) activa(s). Reasígnelas antes de desactivarlo.", n))
			}
		}
		if err := setActive(ctx, tx, id, false, actor.ID, now); err != nil {
			return err
		}
		if err := s.access.RevokeAccess(ctx, tx, id, "deactivated", now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee,
			EntityID: id, Action: audit.ActionDeactivated,
		}, now); err != nil {
			return err
		}
		result, err = findEmployee(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Reactivate turns an inactive account back on.
func (s *Service) Reactivate(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Employee, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	now := s.now()
	var result *Employee

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		employee, err := findEmployee(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if employee == nil {
			return errNotFound
		}
		if employee.IsActive {
			return apperr.Conflict("EMPLOYEE_ALREADY_ACTIVE", "El empleado ya está activo.")
		}
		if err := setActive(ctx, tx, id, true, actor.ID, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee,
			EntityID: id, Action: audit.ActionReactivated,
		}, now); err != nil {
			return err
		}
		result, err = findEmployee(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResendActivation issues a new activation link to an employee who has not set a password yet.
func (s *Service) ResendActivation(ctx context.Context, actor auth.Actor, id uuid.UUID) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if _, err := s.activation.IssueActivation(ctx, id, &actor.ID); err != nil {
		return err
	}
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee,
			EntityID: id, Action: audit.ActionUpdated, Changes: map[string]any{"activationLink": "reissued"},
		}, s.now())
	})
}

// Delete permanently removes an employee who never acted or signed in. It is exceptional: anyone with
// history is deactivated instead.
func (s *Service) Delete(ctx context.Context, actor auth.Actor, id uuid.UUID, reason string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo.").
			WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio."})
	}
	if id == actor.ID {
		return apperr.Conflict("CANNOT_DELETE_SELF", "No puede eliminar su propia cuenta.")
	}
	now := s.now()
	hasHistory := apperr.Conflict("EMPLOYEE_HAS_HISTORY",
		"El empleado tiene historial y no puede eliminarse. Desactívelo en su lugar.")

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		employee, err := findEmployee(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if employee == nil {
			return errNotFound
		}
		logged, err := hasLoggedHistory(ctx, tx, id)
		if err != nil {
			return err
		}
		if logged {
			return hasHistory
		}
		if err := s.access.PurgeTokens(ctx, tx, id); err != nil {
			return err
		}
		if err := deleteEmployee(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityEmployee,
			EntityID: id, Action: audit.ActionDeleted, Reason: reason,
			Changes: map[string]any{
				"documentType": employee.DocumentType, "documentNumber": employee.DocumentNumber,
				"firstName": employee.FirstName, "lastName": employee.LastName, "role": employee.Role,
			},
		}, now)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return hasHistory // other records still reference the employee
	}
	return err
}
