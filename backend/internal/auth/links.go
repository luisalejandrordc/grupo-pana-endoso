package auth

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Only the SHA-256 of a link's token is stored, so a link cannot be rebuilt from the database later.
// The emails therefore travel in two steps:
//
//  1. The business action (creating an employee, creating a customer, asking for a resend or reset)
//      queues a notification that says "send an activation email to this account". It carries no token.
//  2. The notification dispatcher, at the moment it sends, calls IssueLink in the same transaction as the
//     send. IssueLink cancels any open link of that account and mints a fresh one.
//
// The 72-hour or 1-hour clock starts when the email actually goes out, not when it was queued,
// so a delayed or retried email never arrives already expired.

var errNoNotifier = errors.New("auth: no notifier is configured, so the email cannot be queued")

// LinkKind says what a single-use link is for.
type LinkKind string

// The kinds of link. Their values are the token_purpose enum of the database.
const (
	LinkActivation    LinkKind = purposeActivation
	LinkPasswordReset LinkKind = purposePasswordReset
)

// LinkRecipient is the account a link is for. Exactly one field is set.
type LinkRecipient struct {
	EmployeeID *uuid.UUID
	CustomerID *uuid.UUID
}

func (r LinkRecipient) owner() (accountOwner, bool) {
	if (r.EmployeeID == nil) == (r.CustomerID == nil) {
		return accountOwner{}, false
	}
	return accountOwner{EmployeeID: r.EmployeeID, CustomerID: r.CustomerID}, true
}

// AccountLink is a freshly minted single-use link.
type AccountLink struct {
	URL       string
	ExpiresIn time.Duration // how long it stays valid, counted from now
}

// linkURL builds the address of a link.
func (s *Service) linkURL(kind LinkKind, owner accountOwner, plain string) string {
	base := s.baseURL
	if owner.CustomerID != nil {
		base = s.portalBaseURL
	}
	if kind == LinkPasswordReset {
		return base + "/reset-password?token=" + plain
	}
	return base + "/activate-account?token=" + plain
}

func linkTTL(kind LinkKind) time.Duration {
	if kind == LinkPasswordReset {
		return resetTTL
	}
	return activationTTL
}

// IssueLink cancels the open links of this kind for the account and mints a new one, inside tx. The returned
// string is empty when a link was issued; when it is not empty it says (in Spanish, for the notification
// log) why no email is needed any more: the account was deleted, deactivated or has already been activated.
func (s *Service) IssueLink(ctx context.Context, tx pgx.Tx, kind LinkKind, to LinkRecipient) (AccountLink, string, error) {
	owner, ok := to.owner()
	if !ok {
		return AccountLink{}, "", errors.New("auth: a link needs exactly one recipient")
	}
	now := s.now()

	if owner.CustomerID != nil {
		if kind != LinkActivation {
			return AccountLink{}, "", errors.New("auth: customers only have activation links")
		}
		customer, err := findCustomerByID(ctx, tx, *owner.CustomerID, true)
		if err != nil {
			return AccountLink{}, "", err
		}
		switch {
		case customer == nil:
			return AccountLink{}, "el cliente ya no existe", nil
		case !customer.IsActive:
			return AccountLink{}, "el cliente está desactivado", nil
		case customer.ActivatedAt != nil:
			return AccountLink{}, "el cliente ya activó su cuenta", nil
		}
		link, err := s.mint(ctx, tx, kind, owner, now)
		if err != nil {
			return AccountLink{}, "", err
		}
		return link, "", markCustomerInvited(ctx, tx, customer.ID, now)
	}

	emp, err := findEmployeeByID(ctx, tx, *owner.EmployeeID, true)
	if err != nil {
		return AccountLink{}, "", err
	}
	switch {
	case emp == nil || emp.IsSystem:
		return AccountLink{}, "el empleado ya no existe", nil
	case !emp.IsActive:
		return AccountLink{}, "el empleado está desactivado", nil
	case kind == LinkActivation && emp.ActivatedAt != nil:
		return AccountLink{}, "la cuenta ya fue activada", nil
	case kind == LinkPasswordReset && emp.PasswordHash == nil:
		return AccountLink{}, "la cuenta todavía no tiene contraseña", nil
	}
	link, err := s.mint(ctx, tx, kind, owner, now)
	return link, "", err
}

func (s *Service) mint(ctx context.Context, tx pgx.Tx, kind LinkKind, owner accountOwner, now time.Time) (AccountLink, error) {
	if err := revokeOpenAccountTokens(ctx, tx, owner.id(), string(kind), now); err != nil {
		return AccountLink{}, err
	}
	plain, hash, err := NewToken()
	if err != nil {
		return AccountLink{}, err
	}
	ttl := linkTTL(kind)
	if err := insertAccountToken(ctx, tx, ids.New(), owner, string(kind), hash, now.Add(ttl), nil, now); err != nil {
		return AccountLink{}, err
	}
	return AccountLink{URL: s.linkURL(kind, owner, plain), ExpiresIn: ttl}, nil
}

// CancelLinks revokes the open links of this kind for the account. The dispatcher calls it when an email
// that carried a link could not be delivered, so a link nobody received does not stay valid.
func (s *Service) CancelLinks(ctx context.Context, tx pgx.Tx, kind LinkKind, to LinkRecipient) error {
	owner, ok := to.owner()
	if !ok {
		return errors.New("auth: a link needs exactly one recipient")
	}
	return revokeOpenAccountTokens(ctx, tx, owner.id(), string(kind), s.now())
}

// QueueActivation queues the activation email of an employee who has not set a password yet, inside tx. Staff
// management calls it in the same transaction that creates the employee (so the email exists if and only if the
// employee does) and again to resend it.
func (s *Service) QueueActivation(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID) error {
	emp, err := findEmployeeByID(ctx, tx, employeeID, true)
	if err != nil {
		return err
	}
	if emp == nil || !emp.IsActive || emp.IsSystem {
		return apperr.NotFound("EMPLOYEE_NOT_FOUND", "El empleado no existe o está inactivo.")
	}
	if emp.ActivatedAt != nil {
		return apperr.Conflict("EMPLOYEE_ALREADY_ACTIVATED", "La cuenta ya fue activada.")
	}
	if s.notifier == nil {
		return errNoNotifier
	}
	return s.notifier.EmployeeActivation(ctx, tx, emp.ID, emp.BusinessEmail)
}

// MintActivationLink creates an activation link and returns it without sending anything. It is for the
// first-Admin command, which runs where no email can be relied on and prints the link for the operator.
func (s *Service) MintActivationLink(ctx context.Context, employeeID uuid.UUID) (string, error) {
	var link AccountLink
	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		issued, skipped, err := s.IssueLink(ctx, tx, LinkActivation, LinkRecipient{EmployeeID: &employeeID})
		if err != nil {
			return err
		}
		if skipped != "" {
			return apperr.Conflict("ACTIVATION_NOT_NEEDED", skipped)
		}
		link = issued
		return nil
	})
	return link.URL, err
}
