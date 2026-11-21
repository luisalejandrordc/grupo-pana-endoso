package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// querier is satisfied by both the connection pool and a transaction, so the same query functions
// work inside and outside transactions.
//
// The queries are written by hand for now. Enum parameters are sent as text and cast in SQL
// ($1::text::some_enum), and enum columns are read back with ::text.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// employeeRecord is the part of an employee that authentication needs.
type employeeRecord struct {
	ID             uuid.UUID
	DocumentType   string
	DocumentNumber string
	FirstName      string
	LastName       string
	BusinessEmail  string
	Role           Role
	PasswordHash   *string
	ActivatedAt    *time.Time
	IsActive       bool
	IsSystem       bool
	HasPhoto       bool
	Login          LoginState
}

const employeeColumns = `id, document_type::text, document_number, first_name, last_name, business_email,
	role::text, password_hash, activated_at, is_active, is_system,
	failed_login_count, first_failed_login_at, locked_until, profile_picture_key IS NOT NULL`

// scanEmployee returns (nil, nil) when the row does not exist.
func scanEmployee(row pgx.Row) (*employeeRecord, error) {
	var e employeeRecord
	var role string
	err := row.Scan(&e.ID, &e.DocumentType, &e.DocumentNumber, &e.FirstName, &e.LastName, &e.BusinessEmail,
		&role, &e.PasswordHash, &e.ActivatedAt, &e.IsActive, &e.IsSystem,
		&e.Login.FailedCount, &e.Login.WindowStartedAt, &e.Login.LockedUntil, &e.HasPhoto)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Role = Role(role)
	return &e, nil
}

// findEmployeeByDocument looks an employee up by document. The system actor is never returned.
func findEmployeeByDocument(ctx context.Context, q querier, docType, docNumber string, forUpdate bool) (*employeeRecord, error) {
	sql := `SELECT ` + employeeColumns + ` FROM employees
		WHERE document_type = $1::text::employee_document_type AND document_number = $2 AND NOT is_system`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanEmployee(q.QueryRow(ctx, sql, docType, docNumber))
}

func findEmployeeByID(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*employeeRecord, error) {
	sql := `SELECT ` + employeeColumns + ` FROM employees WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanEmployee(q.QueryRow(ctx, sql, id))
}

func updateLoginState(ctx context.Context, q querier, id uuid.UUID, state LoginState) error {
	_, err := q.Exec(ctx, `UPDATE employees
		SET failed_login_count = $2, first_failed_login_at = $3, locked_until = $4
		WHERE id = $1`, id, state.FailedCount, state.WindowStartedAt, state.LockedUntil)
	return err
}

// setPassword stores a new password hash, marks the account activated and clears the lockout.
func setPassword(ctx context.Context, q querier, id uuid.UUID, hash string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE employees
		SET password_hash = $2, activated_at = COALESCE(activated_at, $3),
			failed_login_count = 0, first_failed_login_at = NULL, locked_until = NULL,
			updated_by = $1, updated_at = $3, row_version = row_version + 1
		WHERE id = $1`, id, hash, now)
	return err
}

// ---- refresh tokens ----------------------------------------------------------------------------

type refreshToken struct {
	ID                uuid.UUID
	EmployeeID        uuid.UUID
	FamilyID          uuid.UUID
	ParentID          *uuid.UUID
	RememberMe        bool
	CreatedAt         time.Time
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	RotatedAt         *time.Time
	RevokedAt         *time.Time
}

func insertRefreshToken(ctx context.Context, q querier, t refreshToken, tokenHash string) error {
	_, err := q.Exec(ctx, `INSERT INTO employee_refresh_tokens
		(id, employee_id, token_hash, family_id, parent_token_id, remember_me,
		 created_at, expires_at, absolute_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.EmployeeID, tokenHash, t.FamilyID, t.ParentID, t.RememberMe,
		t.CreatedAt, t.ExpiresAt, t.AbsoluteExpiresAt)
	return err
}

// findRefreshTokenByHash locks the row so concurrent refreshes of the same token are serialized.
// It returns (nil, nil) when no token matches.
func findRefreshTokenByHash(ctx context.Context, q querier, tokenHash string) (*refreshToken, error) {
	var t refreshToken
	err := q.QueryRow(ctx, `SELECT id, employee_id, family_id, parent_token_id, remember_me,
			created_at, expires_at, absolute_expires_at, rotated_at, revoked_at
		FROM employee_refresh_tokens WHERE token_hash = $1 FOR UPDATE`, tokenHash).
		Scan(&t.ID, &t.EmployeeID, &t.FamilyID, &t.ParentID, &t.RememberMe,
			&t.CreatedAt, &t.ExpiresAt, &t.AbsoluteExpiresAt, &t.RotatedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func markRefreshRotated(ctx context.Context, q querier, id uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE employee_refresh_tokens SET rotated_at = $2 WHERE id = $1`, id, now)
	return err
}

func revokeRefreshFamily(ctx context.Context, q querier, familyID uuid.UUID, reason string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE employee_refresh_tokens
		SET revoked_at = $3, revoked_reason = $2::text::revoked_reason
		WHERE family_id = $1 AND revoked_at IS NULL`, familyID, reason, now)
	return err
}

func revokeAllRefreshTokens(ctx context.Context, q querier, employeeID uuid.UUID, reason string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE employee_refresh_tokens
		SET revoked_at = $3, revoked_reason = $2::text::revoked_reason
		WHERE employee_id = $1 AND revoked_at IS NULL`, employeeID, reason, now)
	return err
}

// ---- activation and reset links ----------------------------------------------------------------

const (
	purposeActivation    = "activation"
	purposePasswordReset = "password_reset"
)

// accountOwner is who an account token belongs to: exactly one of the two is set.
type accountOwner struct {
	EmployeeID *uuid.UUID
	CustomerID *uuid.UUID
}

func (o accountOwner) id() uuid.UUID {
	if o.EmployeeID != nil {
		return *o.EmployeeID
	}
	return *o.CustomerID
}

type accountToken struct {
	ID        uuid.UUID
	Owner     accountOwner
	Purpose   string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

func insertAccountToken(ctx context.Context, q querier, id uuid.UUID, owner accountOwner, purpose,
	tokenHash string, expiresAt time.Time, createdBy *uuid.UUID, now time.Time,
) error {
	_, err := q.Exec(ctx, `INSERT INTO account_tokens
		(id, purpose, employee_id, customer_id, token_hash, expires_at, created_by, created_at)
		VALUES ($1, $2::text::token_purpose, $3, $4, $5, $6, $7, $8)`,
		id, purpose, owner.EmployeeID, owner.CustomerID, tokenHash, expiresAt, createdBy, now)
	return err
}

// revokeOpenAccountTokens cancels the unused links of one purpose.
func revokeOpenAccountTokens(ctx context.Context, q querier, ownerID uuid.UUID, purpose string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE account_tokens SET revoked_at = $3
		WHERE (employee_id = $1 OR customer_id = $1) AND purpose = $2::text::token_purpose
		  AND used_at IS NULL AND revoked_at IS NULL`, ownerID, purpose, now)
	return err
}

// findAccountTokenByHash returns (nil, nil) when no link matches. The link may belong to an employee or a customer.
func findAccountTokenByHash(ctx context.Context, q querier, tokenHash string) (*accountToken, error) {
	var t accountToken
	err := q.QueryRow(ctx, `SELECT id, employee_id, customer_id, purpose::text, expires_at, used_at, revoked_at
		FROM account_tokens WHERE token_hash = $1 FOR UPDATE`, tokenHash).
		Scan(&t.ID, &t.Owner.EmployeeID, &t.Owner.CustomerID, &t.Purpose, &t.ExpiresAt, &t.UsedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func markAccountTokenUsed(ctx context.Context, q querier, id uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE account_tokens SET used_at = $2 WHERE id = $1`, id, now)
	return err
}

// ---- customers (only what their activation link needs) -----------------------------------------

// customerRecord is the part of a customer that activation needs.
type customerRecord struct {
	ID             uuid.UUID
	Email          string
	DocumentNumber string
	IsActive       bool
	ActivatedAt    *time.Time
}

// findCustomerByID returns (nil, nil) when the customer does not exist.
func findCustomerByID(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*customerRecord, error) {
	sql := `SELECT id, email, document_number, is_active, activated_at FROM customers WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	var c customerRecord
	err := q.QueryRow(ctx, sql, id).Scan(&c.ID, &c.Email, &c.DocumentNumber, &c.IsActive, &c.ActivatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// setCustomerPassword stores a customer's first password and clears the lockout.
func setCustomerPassword(ctx context.Context, q querier, id uuid.UUID, hash string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customers
		SET password_hash = $2, activated_at = COALESCE(activated_at, $3),
			failed_login_count = 0, first_failed_login_at = NULL, locked_until = NULL, updated_at = $3
		WHERE id = $1`, id, hash, now)
	return err
}

// latestPasswordResetQueuedAt tells when the last reset email was queued for an employee, which is what the
// once-a-minute limit counts from.
func latestPasswordResetQueuedAt(ctx context.Context, q querier, employeeID uuid.UUID) (*time.Time, error) {
	var latest *time.Time
	err := q.QueryRow(ctx, `SELECT max(created_at) FROM notification_logs
		WHERE recipient_employee_id = $1 AND notification_type = 'password_reset'`, employeeID).Scan(&latest)
	return latest, err
}

// ---- access log --------------------------------------------------------------------------------

type accessLogRow struct {
	ID             uuid.UUID
	EmployeeID     *uuid.UUID
	CustomerID     *uuid.UUID
	DocumentType   string
	DocumentNumber string
	Action         string
	FailureReason  string
	DeviceInfo     string
	IP             string
	CreatedAt      time.Time
}

func insertAccessLog(ctx context.Context, q querier, row accessLogRow) error {
	account := "employee"
	if row.CustomerID != nil {
		account = "customer"
	}
	_, err := q.Exec(ctx, `INSERT INTO access_logs
		(id, account_type, employee_id, customer_id, attempted_document_type, attempted_document_number,
		 action_type, failure_reason, device_info, ip_address, created_at)
		VALUES ($1, $2::text::account_type, $3, $4, $5, $6, $7::text::access_action,
				$8::text::access_failure_reason, $9, $10, $11)`,
		row.ID, account, row.EmployeeID, row.CustomerID, nilIfEmpty(row.DocumentType), nilIfEmpty(row.DocumentNumber),
		row.Action, nilIfEmpty(row.FailureReason), nilIfEmpty(row.DeviceInfo), nilIfEmpty(row.IP), row.CreatedAt)
	return err
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
