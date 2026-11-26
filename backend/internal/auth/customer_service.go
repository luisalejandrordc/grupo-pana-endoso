package auth

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The customer portal's authentication. It follows the employee flow in service.go step by step (same lockout,
// same timing protection, same rotation and reuse detection), over the customer tables, and issues tokens that
// only the customer routes accept.

// CustomerProfile is what the portal needs to greet the signed-in customer.
type CustomerProfile struct {
	ID             uuid.UUID `json:"id"`
	DisplayName    string    `json:"displayName"`
	DocumentType   string    `json:"documentType"`
	DocumentNumber string    `json:"documentNumber"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
}

func customerProfileOf(c *customerAccount) CustomerProfile {
	return CustomerProfile{
		ID: c.ID, DisplayName: c.DisplayName, DocumentType: c.DocumentType, DocumentNumber: c.DocumentNumber,
		Email: c.Email, Phone: c.Phone,
	}
}

// CustomerSession is the result of a customer's login or refresh.
type CustomerSession struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string // empty when the browser's cookie must stay as it is (grace replay)
	RefreshExpiresAt time.Time
	RememberMe       bool
	Customer         CustomerProfile
}

// CustomerActor is the authenticated customer behind a request.
type CustomerActor struct {
	ID uuid.UUID
}

// customerDocumentPatterns mirror the customers_document_format constraint of the database.
var customerDocumentPatterns = map[string]*regexp.Regexp{
	"DNI": regexp.MustCompile(`^[0-9]{8}$`),
	"RUC": regexp.MustCompile(`^[0-9]{11}$`),
	"CE":  regexp.MustCompile(`^[A-Z0-9]{9}$`),
	"PAS": regexp.MustCompile(`^[A-Z0-9]{6,12}$`),
	"CDI": regexp.MustCompile(`^[A-Z0-9]{8,12}$`),
}

// NormalizeCustomerIdentifier trims and upper-cases a customer's document and checks its format. Customers can
// sign in with any of their five document types, unlike employees.
func NormalizeCustomerIdentifier(docType, docNumber string) (string, string, bool) {
	t := strings.ToUpper(strings.TrimSpace(docType))
	n := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(docNumber), " ", ""))
	pattern, known := customerDocumentPatterns[t]
	return t, n, known && pattern.MatchString(n)
}

// CustomerLogin checks the credentials and starts a session. Like the employees', every failure looks the same to
// the caller (unknown account, wrong password, locked, inactive, not yet activated); the reason goes to the log.
func (s *Service) CustomerLogin(ctx context.Context, req LoginRequest, client ClientInfo) (*CustomerSession, error) {
	docType, docNumber, valid := NormalizeCustomerIdentifier(req.DocumentType, req.DocumentNumber)
	now := s.now()

	var session *CustomerSession
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var customer *customerAccount
		if valid {
			found, err := findCustomerAccountByDocument(ctx, tx, docType, docNumber, true)
			if err != nil {
				return err
			}
			customer = found
		}

		reason := ""
		switch {
		case customer == nil:
			reason = "invalid_credentials"
		case !customer.IsActive:
			reason = "account_inactive"
		case customer.PasswordHash == nil:
			reason = "not_activated"
		case customer.Login.IsLocked(now):
			reason = "account_locked"
		}

		entry := accessEntry{DocType: docType, DocNumber: docNumber, Action: "login_failed"}
		if customer != nil {
			entry.CustomerID = &customer.ID
		}
		if reason != "" {
			// Spend the same time as a real check so timing does not reveal the account state.
			SpendPasswordCheckTime(req.Password)
			entry.Failure = reason
			failure = errInvalidCredentials
			return s.logAccess(ctx, tx, entry, client, now)
		}

		matches, err := VerifyPassword(req.Password, *customer.PasswordHash)
		if err != nil {
			return fmt.Errorf("stored password hash is unusable for customer %s: %w", customer.ID, err)
		}
		if !matches {
			if err := updateCustomerLoginState(ctx, tx, customer.ID, customer.Login.AfterFailure(now)); err != nil {
				return err
			}
			entry.Failure = "invalid_credentials"
			failure = errInvalidCredentials
			return s.logAccess(ctx, tx, entry, client, now)
		}

		if customer.Login.FailedCount != 0 || customer.Login.LockedUntil != nil || customer.Login.WindowStartedAt != nil {
			if err := updateCustomerLoginState(ctx, tx, customer.ID, customer.Login.AfterSuccess()); err != nil {
				return err
			}
		}
		started, err := s.startCustomerSession(ctx, tx, customer, req.RememberMe, now)
		if err != nil {
			return err
		}
		session = started
		entry.Action = "login_success"
		return s.logAccess(ctx, tx, entry, client, now)
	})
	if err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return session, nil
}

func (s *Service) startCustomerSession(ctx context.Context, q querier, c *customerAccount, rememberMe bool, now time.Time) (*CustomerSession, error) {
	plain, hash, err := NewToken()
	if err != nil {
		return nil, err
	}
	expires, absolute := sessionExpiry(rememberMe, now)
	token := customerRefreshToken{
		ID: ids.New(), CustomerID: c.ID, FamilyID: ids.New(), RememberMe: rememberMe,
		CreatedAt: now, ExpiresAt: expires, AbsoluteExpiresAt: absolute,
	}
	if err := insertCustomerRefreshToken(ctx, q, token, hash); err != nil {
		return nil, err
	}
	access, accessExpires, err := IssueCustomerAccessToken(s.secret, c.ID, now)
	if err != nil {
		return nil, err
	}
	return &CustomerSession{
		AccessToken: access, AccessExpiresAt: accessExpires,
		RefreshToken: plain, RefreshExpiresAt: expires, RememberMe: rememberMe,
		Customer: customerProfileOf(c),
	}, nil
}

// CustomerRefresh exchanges a refresh token for a new access token and a rotated refresh token. Replaying an
// already-rotated token revokes the whole session family, except inside the short grace window, where only a new
// access token is returned.
func (s *Service) CustomerRefresh(ctx context.Context, plain string) (*CustomerSession, error) {
	if plain == "" {
		return nil, errSessionInvalid
	}
	now := s.now()

	var session *CustomerSession
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findCustomerRefreshTokenByHash(ctx, tx, HashToken(plain))
		if err != nil {
			return err
		}
		if token == nil || token.RevokedAt != nil ||
			!now.Before(token.ExpiresAt) || !now.Before(token.AbsoluteExpiresAt) {
			failure = errSessionInvalid
			return nil
		}

		customer, err := findCustomerAccountByID(ctx, tx, token.CustomerID, false)
		if err != nil {
			return err
		}
		if customer == nil || !customer.IsActive || customer.PasswordHash == nil {
			failure = errSessionInvalid
			return revokeCustomerRefreshFamily(ctx, tx, token.FamilyID, "deactivated", now)
		}

		if token.RotatedAt != nil {
			if now.Sub(*token.RotatedAt) <= refreshGrace {
				// Two tabs refreshed at once: the other one already holds the new refresh token.
				access, accessExpires, err := IssueCustomerAccessToken(s.secret, customer.ID, now)
				if err != nil {
					return err
				}
				session = &CustomerSession{
					AccessToken: access, AccessExpiresAt: accessExpires,
					RememberMe: token.RememberMe, Customer: customerProfileOf(customer),
				}
				return nil
			}
			failure = errSessionInvalid // a stolen or replayed token: kill the whole family
			return revokeCustomerRefreshFamily(ctx, tx, token.FamilyID, "reuse_detected", now)
		}

		newPlain, newHash, err := NewToken()
		if err != nil {
			return err
		}
		expires := token.AbsoluteExpiresAt
		if token.RememberMe {
			expires = earlier(now.Add(refreshRememberTTL), token.AbsoluteExpiresAt)
		}
		if err := markCustomerRefreshRotated(ctx, tx, token.ID, now); err != nil {
			return err
		}
		parent := token.ID
		next := customerRefreshToken{
			ID: ids.New(), CustomerID: customer.ID, FamilyID: token.FamilyID, ParentID: &parent,
			RememberMe: token.RememberMe, CreatedAt: now, ExpiresAt: expires,
			AbsoluteExpiresAt: token.AbsoluteExpiresAt,
		}
		if err := insertCustomerRefreshToken(ctx, tx, next, newHash); err != nil {
			return err
		}
		access, accessExpires, err := IssueCustomerAccessToken(s.secret, customer.ID, now)
		if err != nil {
			return err
		}
		session = &CustomerSession{
			AccessToken: access, AccessExpiresAt: accessExpires,
			RefreshToken: newPlain, RefreshExpiresAt: expires, RememberMe: token.RememberMe,
			Customer: customerProfileOf(customer),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return session, nil
}

// CustomerLogout ends the session the refresh token belongs to. It never fails for an unknown token.
func (s *Service) CustomerLogout(ctx context.Context, plain string, client ClientInfo) error {
	if plain == "" {
		return nil
	}
	now := s.now()
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findCustomerRefreshTokenByHash(ctx, tx, HashToken(plain))
		if err != nil || token == nil {
			return err
		}
		if err := revokeCustomerRefreshFamily(ctx, tx, token.FamilyID, "logout", now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{CustomerID: &token.CustomerID, Action: "logout"}, client, now)
	})
}

// AuthenticateCustomer verifies a customer's access token and loads the customer from the database, so a
// deactivation applies immediately.
func (s *Service) AuthenticateCustomer(ctx context.Context, token string) (CustomerActor, error) {
	id, err := ParseCustomerAccessToken(s.secret, token, s.now())
	if err != nil {
		return CustomerActor{}, errAccessTokenInvalid
	}
	customer, err := findCustomerAccountByID(ctx, s.db, id, false)
	if err != nil {
		return CustomerActor{}, err
	}
	if customer == nil || !customer.IsActive || customer.PasswordHash == nil {
		return CustomerActor{}, errAccessTokenInvalid
	}
	return CustomerActor{ID: customer.ID}, nil
}

// CustomerMe returns the signed-in customer's profile.
func (s *Service) CustomerMe(ctx context.Context, id uuid.UUID) (*CustomerProfile, error) {
	customer, err := findCustomerAccountByID(ctx, s.db, id, false)
	if err != nil {
		return nil, err
	}
	if customer == nil || !customer.IsActive {
		return nil, errAccessTokenInvalid
	}
	profile := customerProfileOf(customer)
	return &profile, nil
}

// ChangeCustomerPassword changes the password of a signed-in customer and ends every one of their sessions.
func (s *Service) ChangeCustomerPassword(ctx context.Context, customerID uuid.UUID, current, next string, client ClientInfo) error {
	now := s.now()
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		customer, err := findCustomerAccountByID(ctx, tx, customerID, true)
		if err != nil {
			return err
		}
		if customer == nil || !customer.IsActive {
			failure = errAccessTokenInvalid
			return nil
		}
		if customer.PasswordHash == nil {
			failure = apperr.Unprocessable("PASSWORD_NOT_SET",
				"Su cuenta aún no ha sido activada. Use el enlace de activación que le enviamos para definir su contraseña.")
			return nil
		}
		matches, err := VerifyPassword(current, *customer.PasswordHash)
		if err != nil {
			return err
		}
		if !matches {
			failure = apperr.Unprocessable("INVALID_CURRENT_PASSWORD", "La contraseña actual no es correcta.").
				WithFields(apperr.FieldError{
					Field: "currentPassword", Code: "INVALID_CURRENT_PASSWORD",
					Message: "La contraseña actual no es correcta.",
				})
			return nil
		}
		if problem := ValidatePassword(next, customer.DocumentNumber); problem != nil {
			failure = problem
			return nil
		}
		hash, err := HashPassword(next)
		if err != nil {
			return err
		}
		if err := setCustomerPassword(ctx, tx, customer.ID, hash, now); err != nil {
			return err
		}
		if err := revokeAllCustomerRefreshTokens(ctx, tx, customer.ID, "password_reset", now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{
			CustomerID: &customer.ID, DocType: customer.DocumentType,
			DocNumber: customer.DocumentNumber, Action: "password_changed",
		}, client, now)
	})
	if err != nil {
		return err
	}
	if failure != nil {
		return failure
	}
	return nil
}

// CustomerForgotPassword queues a reset email when the customer exists, is active and has a password. It always
// succeeds from the caller's point of view, so it cannot be used to discover which documents exist.
func (s *Service) CustomerForgotPassword(ctx context.Context, docType, docNumber string, client ClientInfo) error {
	docType, docNumber, valid := NormalizeCustomerIdentifier(docType, docNumber)
	if !valid {
		return nil
	}
	now := s.now()

	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		customer, err := findCustomerAccountByDocument(ctx, tx, docType, docNumber, true)
		if err != nil {
			return err
		}
		if customer == nil || !customer.IsActive || customer.PasswordHash == nil {
			return nil
		}
		last, err := latestCustomerPasswordResetQueuedAt(ctx, tx, customer.ID)
		if err != nil {
			return err
		}
		if last != nil && now.Sub(*last) < resetCooldown {
			return nil
		}
		if s.notifier == nil {
			return errNoNotifier
		}
		if err := s.notifier.CustomerPasswordReset(ctx, tx, customer.ID, customer.Email); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{
			CustomerID: &customer.ID, DocType: docType,
			DocNumber: docNumber, Action: "password_reset_requested",
		}, client, now)
	})
}

// RevokeCustomerAccess ends every session and cancels every open activation or reset link of a customer.
// Customer management calls it inside its own transaction when a customer is deactivated.
func (s *Service) RevokeCustomerAccess(ctx context.Context, tx pgx.Tx, customerID uuid.UUID, reason string, now time.Time) error {
	if err := revokeAllCustomerRefreshTokens(ctx, tx, customerID, reason, now); err != nil {
		return err
	}
	if err := revokeOpenAccountTokens(ctx, tx, customerID, purposeActivation, now); err != nil {
		return err
	}
	return revokeOpenAccountTokens(ctx, tx, customerID, purposePasswordReset, now)
}

// resetCustomerPassword is ResetPassword for a link that belongs to a customer. It returns the problem to show the
// caller (nil on success) separately from a database error.
func (s *Service) resetCustomerPassword(ctx context.Context, tx pgx.Tx, token *accountToken, password string, client ClientInfo, now time.Time) (*apperr.Error, error) {
	customer, err := findCustomerAccountByID(ctx, tx, *token.Owner.CustomerID, true)
	if err != nil {
		return nil, err
	}
	if customer == nil || !customer.IsActive || customer.PasswordHash == nil {
		return errInvalidLink, nil
	}
	if problem := ValidatePassword(password, customer.DocumentNumber); problem != nil {
		return problem, nil // the link stays valid so the person can try another password
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if err := setCustomerPassword(ctx, tx, customer.ID, hash, now); err != nil {
		return nil, err
	}
	if err := markAccountTokenUsed(ctx, tx, token.ID, now); err != nil {
		return nil, err
	}
	if err := revokeAllCustomerRefreshTokens(ctx, tx, customer.ID, "password_reset", now); err != nil {
		return nil, err
	}
	return nil, s.logAccess(ctx, tx, accessEntry{
		CustomerID: &customer.ID, DocType: customer.DocumentType,
		DocNumber: customer.DocumentNumber, Action: "password_reset_completed",
	}, client, now)
}
