package customers

import (
	"context"
	"endosoflow/internal/audit"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	errNotFound        = apperr.NotFound("CUSTOMER_NOT_FOUND", "El cliente no existe.")
	errVersionConflict = apperr.Conflict("VERSION_CONFLICT", "Otro usuario modificó este registro. Recargue e intente nuevamente.")
	errForbidden       = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")
)

const (
	// DefaultLookupsPerMinute is how many lookups one employee may run each minute.
	DefaultLookupsPerMinute = 20
	// DefaultMonthlyQuota is the Codart allowance per calendar month (Lima time).
	DefaultMonthlyQuota = 110
)

// Notifier queues the welcome email of a new customer inside the caller's transaction (implemented by the
// notifications writer). The activation link is created later, when the dispatcher sends the email.
type Notifier interface {
	CustomerCreated(ctx context.Context, tx pgx.Tx, customerID uuid.UUID, email string) error
}

// Deps are the service's dependencies.
type Deps struct {
	DB       auth.Database
	Notifier Notifier         // nil means no welcome email is queued
	Identity IdentityProvider // nil means automatic name lookup is not configured
	Logger   *slog.Logger
	Now      func() time.Time // nil means time.Now

	LookupsPerMinute int // 0 means DefaultLookupsPerMinute
	MonthlyQuota     int // 0 means DefaultMonthlyQuota
}

// Service implements the customer use cases.
type Service struct {
	db               auth.Database
	notifier         Notifier
	identity         IdentityProvider
	logger           *slog.Logger
	now              func() time.Time
	lookupsPerMinute int
	monthlyQuota     int
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{
		db: d.DB, notifier: d.Notifier, identity: d.Identity, logger: d.Logger, now: d.Now,
		lookupsPerMinute: d.LookupsPerMinute, monthlyQuota: d.MonthlyQuota,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.lookupsPerMinute == 0 {
		s.lookupsPerMinute = DefaultLookupsPerMinute
	}
	if s.monthlyQuota == 0 {
		s.monthlyQuota = DefaultMonthlyQuota
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

func seesEveryone(actor auth.Actor) bool {
	return actor.Role == auth.RoleSystemAdmin || actor.Role == auth.RoleSalesManager
}

func requireStaff(actor auth.Actor) error {
	if !isStaff(actor) {
		return errForbidden
	}
	return nil
}

func requireManagement(actor auth.Actor) error {
	if !seesEveryone(actor) {
		return errForbidden
	}
	return nil
}

func requireAdmin(actor auth.Actor) error {
	if actor.Role != auth.RoleSystemAdmin {
		return errForbidden
	}
	return nil
}

func valueOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// canAccess reports whether the actor may see this customer. Customers outside an Advisor's scope answer
// as if they did not exist.
func (s *Service) canAccess(ctx context.Context, q querier, actor auth.Actor, customerID uuid.UUID) (bool, error) {
	if seesEveryone(actor) {
		return true, nil
	}
	return inAdvisorScope(ctx, q, customerID, actor.ID)
}

// List returns one page of customers within the actor's scope.
func (s *Service) List(ctx context.Context, actor auth.Actor, f ListFilter) (*ListResult, error) {
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
	if !seesEveryone(actor) {
		scope = &actor.ID
	}
	items, total, err := listCustomers(ctx, s.db, f, scope)
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Page: f.Page, PageSize: f.PageSize, Total: total}, nil
}

// Get returns one customer the actor may see.
func (s *Service) Get(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Customer, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	customer, err := findCustomer(ctx, s.db, id, false)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, errNotFound
	}
	ok, err := s.canAccess(ctx, s.db, actor, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNotFound
	}
	return customer, nil
}

func rawFromCreate(in CreateInput) rawData {
	return rawData{
		CustomerType: in.CustomerType, DocumentType: in.DocumentType, DocumentNumber: in.DocumentNumber,
		FirstName: in.FirstName, LastName: in.LastName, Gender: in.Gender, LegalName: in.LegalName,
		NameSource: in.NameSource, Nationality: in.Nationality, Phone: in.Phone, Email: in.Email,
		ContactFirstName: in.ContactFirstName, ContactLastName: in.ContactLastName, ContactJobTitle: in.ContactJobTitle,
	}
}

// Create adds a customer and queues their welcome email with an activation link, in the same transaction.
// The creator keeps access until the first sale exists.
func (s *Service) Create(ctx context.Context, actor auth.Actor, in CreateInput) (*Customer, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	data, problem := validateData(rawFromCreate(in))
	if problem != nil {
		return nil, problem
	}
	now := s.now()
	id := ids.New()
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := insertCustomer(ctx, tx, id, data, actor.ID, now); err != nil {
			return err
		}
		if s.notifier != nil {
			if err := s.notifier.CustomerCreated(ctx, tx, id, data.Email); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityCustomer, EntityID: id, Action: audit.ActionCreated,
			Changes: map[string]any{
				"customerType": data.CustomerType, "documentType": data.DocumentType,
				"documentNumber": data.DocumentNumber,
			},
		}, now)
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return findCustomer(ctx, s.db, id, false)
}

func mergeUpdate(current Customer, in UpdateInput) rawData {
	pick := func(newValue *string, old string) string {
		if newValue != nil {
			return *newValue
		}
		return old
	}
	raw := rawData{
		CustomerType:     pick(in.CustomerType, current.CustomerType),
		DocumentType:     pick(in.DocumentType, current.DocumentType),
		DocumentNumber:   pick(in.DocumentNumber, current.DocumentNumber),
		FirstName:        pick(in.FirstName, valueOf(current.FirstName)),
		LastName:         pick(in.LastName, valueOf(current.LastName)),
		Gender:           pick(in.Gender, valueOf(current.Gender)),
		LegalName:        pick(in.LegalName, valueOf(current.LegalName)),
		NameSource:       pick(in.NameSource, current.NameSource),
		Nationality:      pick(in.Nationality, current.Nationality),
		Phone:            pick(in.Phone, current.Phone),
		Email:            pick(in.Email, current.Email),
		ContactFirstName: pick(in.ContactFirstName, valueOf(current.ContactFirstName)),
		ContactLastName:  pick(in.ContactLastName, valueOf(current.ContactLastName)),
		ContactJobTitle:  pick(in.ContactJobTitle, valueOf(current.ContactJobTitle)),
	}
	// A name edited by hand is no longer "looked up", unless the caller says otherwise.
	nameEdited := in.FirstName != nil || in.LastName != nil || in.LegalName != nil
	if nameEdited && in.NameSource == nil {
		raw.NameSource = "manual"
	}
	return raw
}

// diffCustomer summarizes what changed. Contact data is recorded as "changed" without its values.
func diffCustomer(old Customer, d customerData) map[string]any {
	changes := map[string]any{}
	note := func(field, before, after string) {
		if before != after {
			changes[field] = map[string]string{"before": before, "after": after}
		}
	}
	note("customerType", old.CustomerType, d.CustomerType)
	note("documentType", old.DocumentType, d.DocumentType)
	note("documentNumber", old.DocumentNumber, d.DocumentNumber)
	note("firstName", valueOf(old.FirstName), valueOf(d.FirstName))
	note("lastName", valueOf(old.LastName), valueOf(d.LastName))
	note("gender", valueOf(old.Gender), valueOf(d.Gender))
	note("legalName", valueOf(old.LegalName), valueOf(d.LegalName))
	note("nameSource", old.NameSource, d.NameSource)
	note("nationality", old.Nationality, d.Nationality)
	note("contactJobTitle", valueOf(old.ContactJobTitle), valueOf(d.ContactJobTitle))
	for field, pair := range map[string][2]string{
		"phone":            {old.Phone, d.Phone},
		"email":            {old.Email, d.Email},
		"contactFirstName": {valueOf(old.ContactFirstName), valueOf(d.ContactFirstName)},
		"contactLastName":  {valueOf(old.ContactLastName), valueOf(d.ContactLastName)},
	} {
		if pair[0] != pair[1] {
			changes[field] = "changed"
		}
	}
	return changes
}

// Update changes a customer the actor may edit. Existing sale snapshots never change. The caller sends
// the version it read; a stale version is refused.
func (s *Service) Update(ctx context.Context, actor auth.Actor, id uuid.UUID, in UpdateInput) (*Customer, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	if in.Version == nil {
		return nil, apperr.BadRequest("VERSION_REQUIRED", "Falta la versión del registro.")
	}
	now := s.now()
	var result *Customer

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		current, err := findCustomer(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current == nil {
			return errNotFound
		}
		ok, err := s.canAccess(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if !ok {
			return errNotFound
		}
		if current.Version != *in.Version {
			return errVersionConflict
		}
		data, problem := validateData(mergeUpdate(*current, in))
		if problem != nil {
			return problem
		}
		changed, err := updateCustomer(ctx, tx, id, *in.Version, data, actor.ID, now)
		if err != nil {
			return err
		}
		if !changed {
			return errVersionConflict
		}
		if changes := diffCustomer(*current, data); len(changes) > 0 {
			if err := audit.Record(ctx, tx, audit.Event{
				ActorID: &actor.ID, EntityType: audit.EntityCustomer, EntityID: id,
				Action: audit.ActionUpdated, Changes: changes,
			}, now); err != nil {
				return err
			}
		}
		result, err = findCustomer(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return nil, apperr.FromError(err)
	}
	return result, nil
}

func (s *Service) setActiveState(ctx context.Context, actor auth.Actor, id uuid.UUID, active bool) (*Customer, error) {
	if err := requireManagement(actor); err != nil {
		return nil, err
	}
	now := s.now()
	var result *Customer
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		customer, err := findCustomer(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if customer == nil {
			return errNotFound
		}
		if customer.IsActive == active {
			if active {
				return apperr.Conflict("CUSTOMER_ALREADY_ACTIVE", "El cliente ya está activo.")
			}
			return apperr.Conflict("CUSTOMER_ALREADY_INACTIVE", "El cliente ya está inactivo.")
		}
		if err := setActive(ctx, tx, id, active, actor.ID, now); err != nil {
			return err
		}
		action := audit.ActionDeactivated
		if active {
			action = audit.ActionReactivated
		}
		if err := audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityCustomer, EntityID: id, Action: action,
		}, now); err != nil {
			return err
		}
		result, err = findCustomer(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Deactivate stops a customer from receiving new sales or portal access. History is kept.
func (s *Service) Deactivate(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Customer, error) {
	return s.setActiveState(ctx, actor, id, false)
}

// Reactivate turns an inactive customer back on.
func (s *Service) Reactivate(ctx context.Context, actor auth.Actor, id uuid.UUID) (*Customer, error) {
	return s.setActiveState(ctx, actor, id, true)
}

// Delete permanently removes a customer who has no sales. It is exceptional and Admin-only.
func (s *Service) Delete(ctx context.Context, actor auth.Actor, id uuid.UUID, reason string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return apperr.Unprocessable("REASON_REQUIRED", "Debe escribir el motivo.").
			WithFields(apperr.FieldError{Field: "reason", Code: "REQUIRED", Message: "El motivo es obligatorio."})
	}
	now := s.now()
	hasSales := apperr.Conflict("CUSTOMER_HAS_SALES",
		"El cliente tiene ventas y no puede eliminarse. Desactívelo en su lugar.")

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		customer, err := findCustomer(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if customer == nil {
			return errNotFound
		}
		n, err := countSales(ctx, tx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return hasSales
		}
		if err := deleteCustomer(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{
			ActorID: &actor.ID, EntityType: audit.EntityCustomer, EntityID: id,
			Action: audit.ActionDeleted, Reason: reason,
			Changes: map[string]any{
				"customerType": customer.CustomerType, "documentType": customer.DocumentType,
				"documentNumber": customer.DocumentNumber, "displayName": customer.DisplayName,
			},
		}, now)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return hasSales // other records still reference the customer
	}
	return err
}

// Lookup answers whether a customer with this document exists. Admins and Managers get the full record.
// An Advisor who is not linked to the customer gets the legal name only, and never learns about sales.
// Every lookup is logged and each employee has a per-minute limit.
func (s *Service) Lookup(ctx context.Context, actor auth.Actor, docType, docNumber string) (*LookupResult, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}
	docType, docNumber, ok := normalizeDocument(docType, docNumber)
	if !ok {
		return nil, errInvalidDocument
	}
	now := s.now()
	if err := s.checkRate(ctx, actor, []string{"customer_existence"}, "customer_existence", docType, docNumber, now); err != nil {
		return nil, err
	}

	customer, err := findByDocument(ctx, s.db, docType, docNumber)
	if err != nil {
		return nil, err
	}
	outcome := "not_found"
	if customer != nil {
		outcome = "success"
	}
	if err := insertLookupLog(ctx, s.db, ids.New(), actor.ID, "customer_existence", docType, docNumber, outcome, now); err != nil {
		return nil, err
	}
	if customer == nil {
		return &LookupResult{Exists: false}, nil
	}
	linked, err := s.canAccess(ctx, s.db, actor, customer.ID)
	if err != nil {
		return nil, err
	}
	if linked {
		return &LookupResult{Exists: true, Access: "full", Customer: customer}, nil
	}
	return &LookupResult{Exists: true, Access: "name_only", Brief: &Brief{
		ID: customer.ID, DisplayName: customer.DisplayName, CustomerType: customer.CustomerType, IsActive: customer.IsActive,
	}}, nil
}

var errInvalidDocument = apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").
	WithFields(apperr.FieldError{
		Field: "documentNumber", Code: "INVALID_DOCUMENT",
		Message: "El número de documento no es válido para su tipo.",
	})

// checkRate refuses the request when the employee already ran too many lookups in the last minute. The
// refusal is logged too, so the trail shows who was throttled.
func (s *Service) checkRate(ctx context.Context, actor auth.Actor, kinds []string, logKind, docType, docNumber string, now time.Time) error {
	n, err := countRecentLookups(ctx, s.db, actor.ID, kinds, now.Add(-time.Minute))
	if err != nil {
		return err
	}
	if n < s.lookupsPerMinute {
		return nil
	}
	if err := insertLookupLog(ctx, s.db, ids.New(), actor.ID, logKind, docType, docNumber, "rate_limited", now); err != nil {
		return err
	}
	return apperr.TooManyRequests("RATE_LIMITED", "Demasiadas consultas seguidas. Espere un momento e intente nuevamente.")
}
