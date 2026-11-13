package auth

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Session lifetimes and link validity (see the Requirements Catalog, FR-003 to FR-008).
const (
	refreshRememberTTL = 30 * 24 * time.Hour // sliding, for "Mantener sesión iniciada"
	refreshAbsoluteTTL = 90 * 24 * time.Hour // hard cap for remembered sessions
	refreshSessionTTL  = 12 * time.Hour      // without remember-me
	refreshGrace       = 10 * time.Second    // tolerates two tabs refreshing at once
	activationTTL      = 72 * time.Hour
	resetTTL           = time.Hour
	resetCooldown      = time.Minute // at most one reset email per minute per account
)

var (
	errInvalidCredentials = apperr.Unauthorized("INVALID_CREDENTIALS", "Documento o contraseña incorrectos.")
	errSessionInvalid     = apperr.Unauthorized("SESSION_INVALID", "Su sesión no es válida. Inicie sesión nuevamente.")
	errAccessTokenInvalid = apperr.Unauthorized("ACCESS_TOKEN_INVALID", "Su sesión expiró. Inicie sesión nuevamente.")
	errInvalidLink        = apperr.BadRequest("INVALID_LINK", "El enlace no es válido o ya venció.")
)

// Database is what the service needs from the connection pool.
type Database interface {
	database.TxStarter
	querier
}

// Deps are the service's dependencies.
type Deps struct {
	DB         Database
	JWTSecret  string
	AppBaseURL string
	Mail       mail.Sender
	Logger     *slog.Logger
	Now        func() time.Time // nil means time.Now; tests inject a controllable clock
}

// Service implements the employee authentication use cases.
type Service struct {
	db         Database
	secret     []byte
	baseURL    string
	mail       mail.Sender
	logger     *slog.Logger
	now        func() time.Time
	background sync.WaitGroup
}

// NewService builds the service.
func NewService(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		db:      d.DB,
		secret:  []byte(d.JWTSecret),
		baseURL: strings.TrimRight(d.AppBaseURL, "/"),
		mail:    d.Mail,
		logger:  d.Logger,
		now:     now,
	}
}

// WaitForBackgroundWork waits for emails still being sent. Called on shutdown and in tests.
func (s *Service) WaitForBackgroundWork() { s.background.Wait() }

// ClientInfo describes the caller, for the access log.
type ClientInfo struct {
	DeviceInfo string
	IP         string
}

// LoginRequest is the input of Login.
type LoginRequest struct {
	DocumentType   string
	DocumentNumber string
	Password       string
	RememberMe     bool
}

// Profile is the employee data the interface needs after signing in.
type Profile struct {
	ID             uuid.UUID `json:"id"`
	DocumentType   string    `json:"documentType"`
	DocumentNumber string    `json:"documentNumber"`
	FirstName      string    `json:"firstName"`
	LastName       string    `json:"lastName"`
	BusinessEmail  string    `json:"businessEmail"`
	Role           Role      `json:"role"`
}

func profileOf(e *employeeRecord) Profile {
	return Profile{
		ID: e.ID, DocumentType: e.DocumentType, DocumentNumber: e.DocumentNumber,
		FirstName: e.FirstName, LastName: e.LastName, BusinessEmail: e.BusinessEmail, Role: e.Role,
	}
}

// Session is the result of a login or a refresh.
type Session struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string // empty when the browser's cookie must stay as it is (grace replay)
	RefreshExpiresAt time.Time
	RememberMe       bool
	Employee         Profile
}

// Actor is the authenticated employee behind a request. The role comes from the database.
type Actor struct {
	ID   uuid.UUID
	Role Role
}

var (
	dniPattern = regexp.MustCompile(`^[0-9]{8}$`)
	cePattern  = regexp.MustCompile(`^[A-Z0-9]{9}$`)
	pasPattern = regexp.MustCompile(`^[A-Z0-9]{6,12}$`)
)

// NormalizeIdentifier trims and upper-cases an employee's document and checks its format
// (DNI: 8 digits, CE: 9 alphanumeric, PAS: 6 to 12 alphanumeric). The staff module uses it too.
func NormalizeIdentifier(docType, docNumber string) (string, string, bool) {
	t := strings.ToUpper(strings.TrimSpace(docType))
	n := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(docNumber), " ", ""))
	var pattern *regexp.Regexp
	switch t {
	case "DNI":
		pattern = dniPattern
	case "CE":
		pattern = cePattern
	case "PAS":
		pattern = pasPattern
	default:
		return t, n, false
	}
	return t, n, pattern.MatchString(n)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

type accessEntry struct {
	EmployeeID *uuid.UUID
	DocType    string
	DocNumber  string
	Action     string
	Failure    string
}

func (s *Service) logAccess(ctx context.Context, q querier, e accessEntry, client ClientInfo, now time.Time) error {
	return insertAccessLog(ctx, q, accessLogRow{
		ID: ids.New(), EmployeeID: e.EmployeeID,
		DocumentType: truncate(e.DocType, 16), DocumentNumber: truncate(e.DocNumber, 32),
		Action: e.Action, FailureReason: e.Failure,
		DeviceInfo: truncate(client.DeviceInfo, 255), IP: truncate(client.IP, 64), CreatedAt: now,
	})
}

func sessionExpiry(rememberMe bool, now time.Time) (expires, absolute time.Time) {
	if rememberMe {
		return now.Add(refreshRememberTTL), now.Add(refreshAbsoluteTTL)
	}
	end := now.Add(refreshSessionTTL)
	return end, end
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Login checks the credentials and starts a session. Every failure looks the same to the caller
// (unknown account, wrong password, locked, inactive, not yet activated); the reason goes to the log.
func (s *Service) Login(ctx context.Context, req LoginRequest, client ClientInfo) (*Session, error) {
	docType, docNumber, valid := NormalizeIdentifier(req.DocumentType, req.DocumentNumber)
	now := s.now()

	var session *Session
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		var emp *employeeRecord
		if valid {
			found, err := findEmployeeByDocument(ctx, tx, docType, docNumber, true)
			if err != nil {
				return err
			}
			emp = found
		}

		reason := ""
		switch {
		case emp == nil:
			reason = "invalid_credentials"
		case !emp.IsActive:
			reason = "account_inactive"
		case emp.PasswordHash == nil:
			reason = "not_activated"
		case emp.Login.IsLocked(now):
			reason = "account_locked"
		}

		entry := accessEntry{DocType: docType, DocNumber: docNumber, Action: "login_failed"}
		if emp != nil {
			entry.EmployeeID = &emp.ID
		}
		if reason != "" {
			// Spend the same time as a real check so timing does not reveal the account state.
			SpendPasswordCheckTime(req.Password)
			entry.Failure = reason
			failure = errInvalidCredentials
			return s.logAccess(ctx, tx, entry, client, now)
		}

		matches, err := VerifyPassword(req.Password, *emp.PasswordHash)
		if err != nil {
			return fmt.Errorf("stored password hash is unusable for employee %s: %w", emp.ID, err)
		}
		if !matches {
			if err := updateLoginState(ctx, tx, emp.ID, emp.Login.AfterFailure(now)); err != nil {
				return err
			}
			entry.Failure = "invalid_credentials"
			failure = errInvalidCredentials
			return s.logAccess(ctx, tx, entry, client, now)
		}

		if emp.Login.FailedCount != 0 || emp.Login.LockedUntil != nil || emp.Login.WindowStartedAt != nil {
			if err := updateLoginState(ctx, tx, emp.ID, emp.Login.AfterSuccess()); err != nil {
				return err
			}
		}
		started, err := s.startSession(ctx, tx, emp, req.RememberMe, now)
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

func (s *Service) startSession(ctx context.Context, q querier, emp *employeeRecord, rememberMe bool, now time.Time) (*Session, error) {
	plain, hash, err := NewToken()
	if err != nil {
		return nil, err
	}
	expires, absolute := sessionExpiry(rememberMe, now)
	token := refreshToken{
		ID: ids.New(), EmployeeID: emp.ID, FamilyID: ids.New(), RememberMe: rememberMe,
		CreatedAt: now, ExpiresAt: expires, AbsoluteExpiresAt: absolute,
	}
	if err := insertRefreshToken(ctx, q, token, hash); err != nil {
		return nil, err
	}
	access, accessExpires, err := IssueAccessToken(s.secret, emp.ID, emp.Role, now)
	if err != nil {
		return nil, err
	}
	return &Session{
		AccessToken: access, AccessExpiresAt: accessExpires,
		RefreshToken: plain, RefreshExpiresAt: expires, RememberMe: rememberMe,
		Employee: profileOf(emp),
	}, nil
}

// Refresh exchanges a refresh token for a new access token and a rotated refresh token.
// Replaying an already-rotated token revokes the whole session family, except inside the short
// grace window, where only a new access token is returned.
func (s *Service) Refresh(ctx context.Context, plain string) (*Session, error) {
	if plain == "" {
		return nil, errSessionInvalid
	}
	now := s.now()

	var session *Session
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findRefreshTokenByHash(ctx, tx, HashToken(plain))
		if err != nil {
			return err
		}
		if token == nil || token.RevokedAt != nil ||
			!now.Before(token.ExpiresAt) || !now.Before(token.AbsoluteExpiresAt) {
			failure = errSessionInvalid
			return nil
		}

		emp, err := findEmployeeByID(ctx, tx, token.EmployeeID, false)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive || emp.IsSystem {
			failure = errSessionInvalid
			return revokeRefreshFamily(ctx, tx, token.FamilyID, "deactivated", now)
		}

		if token.RotatedAt != nil {
			if now.Sub(*token.RotatedAt) <= refreshGrace {
				// Two tabs refreshed at once: the other one already holds the new refresh token.
				access, accessExpires, err := IssueAccessToken(s.secret, emp.ID, emp.Role, now)
				if err != nil {
					return err
				}
				session = &Session{
					AccessToken: access, AccessExpiresAt: accessExpires,
					RememberMe: token.RememberMe, Employee: profileOf(emp),
				}
				return nil
			}
			failure = errSessionInvalid // a stolen or replayed token: kill the whole family
			return revokeRefreshFamily(ctx, tx, token.FamilyID, "reuse_detected", now)
		}

		newPlain, newHash, err := NewToken()
		if err != nil {
			return err
		}
		expires := token.AbsoluteExpiresAt
		if token.RememberMe {
			expires = earlier(now.Add(refreshRememberTTL), token.AbsoluteExpiresAt)
		}
		if err := markRefreshRotated(ctx, tx, token.ID, now); err != nil {
			return err
		}
		parent := token.ID
		next := refreshToken{
			ID: ids.New(), EmployeeID: emp.ID, FamilyID: token.FamilyID, ParentID: &parent,
			RememberMe: token.RememberMe, CreatedAt: now, ExpiresAt: expires,
			AbsoluteExpiresAt: token.AbsoluteExpiresAt,
		}
		if err := insertRefreshToken(ctx, tx, next, newHash); err != nil {
			return err
		}
		access, accessExpires, err := IssueAccessToken(s.secret, emp.ID, emp.Role, now)
		if err != nil {
			return err
		}
		session = &Session{
			AccessToken: access, AccessExpiresAt: accessExpires,
			RefreshToken: newPlain, RefreshExpiresAt: expires, RememberMe: token.RememberMe,
			Employee: profileOf(emp),
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

// Logout ends the session the refresh token belongs to. It never fails for an unknown token.
func (s *Service) Logout(ctx context.Context, plain string, client ClientInfo) error {
	if plain == "" {
		return nil
	}
	now := s.now()
	return database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findRefreshTokenByHash(ctx, tx, HashToken(plain))
		if err != nil || token == nil {
			return err
		}
		if err := revokeRefreshFamily(ctx, tx, token.FamilyID, "logout", now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{EmployeeID: &token.EmployeeID, Action: "logout"}, client, now)
	})
}

// Authenticate verifies an access token and loads the employee from the database. The role used for
// authorization is the database's, so deactivations and role changes apply immediately.
func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	claims, err := ParseAccessToken(s.secret, token, s.now())
	if err != nil {
		return Actor{}, errAccessTokenInvalid
	}
	emp, err := findEmployeeByID(ctx, s.db, claims.EmployeeID, false)
	if err != nil {
		return Actor{}, err
	}
	if emp == nil || !emp.IsActive || emp.IsSystem {
		return Actor{}, errAccessTokenInvalid
	}
	return Actor{ID: emp.ID, Role: emp.Role}, nil
}

// Me returns the signed-in employee's profile.
func (s *Service) Me(ctx context.Context, id uuid.UUID) (*Profile, error) {
	emp, err := findEmployeeByID(ctx, s.db, id, false)
	if err != nil {
		return nil, err
	}
	if emp == nil || !emp.IsActive {
		return nil, errAccessTokenInvalid
	}
	profile := profileOf(emp)
	return &profile, nil
}

// IssueActivation creates a single-use activation link for an employee who has not set a password yet,
// emails it to their business address and returns it (the first-Admin command prints it).
func (s *Service) IssueActivation(ctx context.Context, employeeID uuid.UUID, createdBy *uuid.UUID) (string, error) {
	now := s.now()
	var link string
	var message mail.Message
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		emp, err := findEmployeeByID(ctx, tx, employeeID, true)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive || emp.IsSystem {
			failure = apperr.NotFound("EMPLOYEE_NOT_FOUND", "El empleado no existe o está inactivo.")
			return nil
		}
		if emp.ActivatedAt != nil {
			failure = apperr.Conflict("EMPLOYEE_ALREADY_ACTIVATED", "La cuenta ya fue activada.")
			return nil
		}
		if err := revokeOpenAccountTokens(ctx, tx, emp.ID, purposeActivation, now); err != nil {
			return err
		}
		plain, hash, err := NewToken()
		if err != nil {
			return err
		}
		if err := insertAccountToken(ctx, tx, ids.New(), emp.ID, purposeActivation, hash,
			now.Add(activationTTL), createdBy, now); err != nil {
			return err
		}
		link = s.baseURL + "/activar-cuenta?token=" + plain
		message = mail.Message{
			To:      emp.BusinessEmail,
			Subject: "Active su cuenta de EndosoFlow",
			Body: fmt.Sprintf("Hola %s,\n\nSe creó su cuenta de EndosoFlow — Grupo Pana. "+
				"Para definir su contraseña, abra este enlace (vigente por 72 horas):\n\n%s\n\n"+
				"Si no esperaba este mensaje, ignórelo.\n", emp.FirstName, link),
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if failure != nil {
		return "", failure
	}
	s.sendAsync(message)
	return link, nil
}

// Activate sets the first password from an activation link.
func (s *Service) Activate(ctx context.Context, plainToken, password string, client ClientInfo) error {
	now := s.now()
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findAccountTokenByHash(ctx, tx, HashToken(plainToken))
		if err != nil {
			return err
		}
		if token == nil || token.Purpose != purposeActivation || token.UsedAt != nil ||
			token.RevokedAt != nil || !now.Before(token.ExpiresAt) {
			failure = errInvalidLink
			return nil
		}
		emp, err := findEmployeeByID(ctx, tx, token.EmployeeID, true)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive || emp.ActivatedAt != nil {
			failure = errInvalidLink
			return nil
		}
		if problem := ValidatePassword(password, emp.DocumentNumber); problem != nil {
			failure = problem // the link stays valid so the person can try another password
			return nil
		}
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		if err := setPassword(ctx, tx, emp.ID, hash, now); err != nil {
			return err
		}
		if err := markAccountTokenUsed(ctx, tx, token.ID, now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{
			EmployeeID: &emp.ID, DocType: emp.DocumentType,
			DocNumber: emp.DocumentNumber, Action: "account_activated",
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

// ForgotPassword emails a reset link when the account exists, is active and has a password. It always
// succeeds from the caller's point of view, so it cannot be used to discover which documents exist.
func (s *Service) ForgotPassword(ctx context.Context, docType, docNumber string, client ClientInfo) error {
	docType, docNumber, valid := NormalizeIdentifier(docType, docNumber)
	if !valid {
		return nil
	}
	now := s.now()
	var message *mail.Message

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		emp, err := findEmployeeByDocument(ctx, tx, docType, docNumber, true)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive || emp.PasswordHash == nil {
			return nil
		}
		last, err := latestAccountTokenCreatedAt(ctx, tx, emp.ID, purposePasswordReset)
		if err != nil {
			return err
		}
		if last != nil && now.Sub(*last) < resetCooldown {
			return nil
		}
		if err := revokeOpenAccountTokens(ctx, tx, emp.ID, purposePasswordReset, now); err != nil {
			return err
		}
		plain, hash, err := NewToken()
		if err != nil {
			return err
		}
		if err := insertAccountToken(ctx, tx, ids.New(), emp.ID, purposePasswordReset, hash,
			now.Add(resetTTL), nil, now); err != nil {
			return err
		}
		message = &mail.Message{
			To:      emp.BusinessEmail,
			Subject: "Restablezca su contraseña de EndosoFlow",
			Body: fmt.Sprintf("Hola %s,\n\nRecibimos una solicitud para restablecer su contraseña. "+
				"Abra este enlace (vigente por 1 hora):\n\n%s/restablecer-contrasena?token=%s\n\n"+
				"Si usted no lo solicitó, ignore este mensaje: su contraseña no cambiará.\n",
				emp.FirstName, s.baseURL, plain),
		}
		return s.logAccess(ctx, tx, accessEntry{
			EmployeeID: &emp.ID, DocType: docType,
			DocNumber: docNumber, Action: "password_reset_requested",
		}, client, now)
	})
	if err != nil {
		return err
	}
	if message != nil {
		s.sendAsync(*message) // in the background, so response time does not reveal whether the account exists
	}
	return nil
}

// ResetPassword sets a new password from a reset link and ends every session of the account.
func (s *Service) ResetPassword(ctx context.Context, plainToken, password string, client ClientInfo) error {
	now := s.now()
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		token, err := findAccountTokenByHash(ctx, tx, HashToken(plainToken))
		if err != nil {
			return err
		}
		if token == nil || token.Purpose != purposePasswordReset || token.UsedAt != nil ||
			token.RevokedAt != nil || !now.Before(token.ExpiresAt) {
			failure = errInvalidLink
			return nil
		}
		emp, err := findEmployeeByID(ctx, tx, token.EmployeeID, true)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive || emp.PasswordHash == nil {
			failure = errInvalidLink
			return nil
		}
		if problem := ValidatePassword(password, emp.DocumentNumber); problem != nil {
			failure = problem
			return nil
		}
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		if err := setPassword(ctx, tx, emp.ID, hash, now); err != nil {
			return err
		}
		if err := markAccountTokenUsed(ctx, tx, token.ID, now); err != nil {
			return err
		}
		if err := revokeAllRefreshTokens(ctx, tx, emp.ID, "password_reset", now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{
			EmployeeID: &emp.ID, DocType: emp.DocumentType,
			DocNumber: emp.DocumentNumber, Action: "password_reset_completed",
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

// ChangePassword lets a signed-in employee change their password. All their sessions end afterwards.
func (s *Service) ChangePassword(ctx context.Context, employeeID uuid.UUID, current, next string, client ClientInfo) error {
	now := s.now()
	var failure *apperr.Error

	err := database.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		emp, err := findEmployeeByID(ctx, tx, employeeID, true)
		if err != nil {
			return err
		}
		if emp == nil || !emp.IsActive {
			failure = errAccessTokenInvalid
			return nil
		}
		if emp.PasswordHash == nil {
			failure = apperr.Unprocessable(
				"PASSWORD_NOT_SET",
				"Tu cuenta aún no ha sido activada. Debes utilizar el enlace de activación para configurar tu contraseña.",
			)
			return nil
		}
		matches, err := VerifyPassword(current, *emp.PasswordHash)
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
		if problem := ValidatePassword(next, emp.DocumentNumber); problem != nil {
			failure = problem
			return nil
		}
		hash, err := HashPassword(next)
		if err != nil {
			return err
		}
		if err := setPassword(ctx, tx, emp.ID, hash, now); err != nil {
			return err
		}
		if err := revokeAllRefreshTokens(ctx, tx, emp.ID, "password_reset", now); err != nil {
			return err
		}
		return s.logAccess(ctx, tx, accessEntry{
			EmployeeID: &emp.ID, DocType: emp.DocumentType,
			DocNumber: emp.DocumentNumber, Action: "password_changed",
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

// sendAsync sends an email without blocking the request. Failures are logged without the recipient or
// the body, because the body can contain a secret link.
func (s *Service) sendAsync(message mail.Message) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, message); err != nil {
			s.logger.Error("sending email failed", "subject", message.Subject, "error", err)
		}
	}()
}

// RevokeAccess ends every session and cancels every open activation or reset link of an employee.
// Staff management calls it inside its own transaction when an account is deactivated.
func (s *Service) RevokeAccess(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID, reason string, now time.Time) error {
	if err := revokeAllRefreshTokens(ctx, tx, employeeID, reason, now); err != nil {
		return err
	}
	if err := revokeOpenAccountTokens(ctx, tx, employeeID, purposeActivation, now); err != nil {
		return err
	}
	return revokeOpenAccountTokens(ctx, tx, employeeID, purposePasswordReset, now)
}

// PurgeTokens deletes every token of an employee who is about to be deleted.
func (s *Service) PurgeTokens(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM employee_refresh_tokens WHERE employee_id = $1`, employeeID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM account_tokens WHERE employee_id = $1`, employeeID)
	return err
}
