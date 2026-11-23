package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The customer's side of authentication: the account itself and its refresh tokens. It mirrors the employee
// code in repository.go, over the customer tables.

// customerAccount is a customer as sign-in needs it.
type customerAccount struct {
	ID             uuid.UUID
	DocumentType   string
	DocumentNumber string
	DisplayName    string
	Email          string
	Phone          string
	PasswordHash   *string
	ActivatedAt    *time.Time
	IsActive       bool
	Login          LoginState
}

const customerAccountColumns = `id, document_type::text, document_number, display_name, email, phone, password_hash,
	activated_at, is_active, failed_login_count, first_failed_login_at, locked_until`

// scanCustomerAccount returns (nil, nil) when the row does not exist.
func scanCustomerAccount(row pgx.Row) (*customerAccount, error) {
	var c customerAccount
	err := row.Scan(&c.ID, &c.DocumentType, &c.DocumentNumber, &c.DisplayName, &c.Email, &c.Phone, &c.PasswordHash,
		&c.ActivatedAt, &c.IsActive, &c.Login.FailedCount, &c.Login.WindowStartedAt, &c.Login.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func findCustomerAccountByDocument(ctx context.Context, q querier, docType, docNumber string, forUpdate bool) (*customerAccount, error) {
	sql := `SELECT ` + customerAccountColumns + ` FROM customers
		WHERE document_type = $1::text::customer_document_type AND document_number = $2`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanCustomerAccount(q.QueryRow(ctx, sql, docType, docNumber))
}

func findCustomerAccountByID(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*customerAccount, error) {
	sql := `SELECT ` + customerAccountColumns + ` FROM customers WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanCustomerAccount(q.QueryRow(ctx, sql, id))
}

func updateCustomerLoginState(ctx context.Context, q querier, id uuid.UUID, state LoginState) error {
	_, err := q.Exec(ctx, `UPDATE customers
		SET failed_login_count = $2, first_failed_login_at = $3, locked_until = $4
		WHERE id = $1`, id, state.FailedCount, state.WindowStartedAt, state.LockedUntil)
	return err
}

// ---- refresh tokens ----------------------------------------------------------------------------

type customerRefreshToken struct {
	ID                uuid.UUID
	CustomerID        uuid.UUID
	FamilyID          uuid.UUID
	ParentID          *uuid.UUID
	RememberMe        bool
	CreatedAt         time.Time
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	RotatedAt         *time.Time
	RevokedAt         *time.Time
}

func insertCustomerRefreshToken(ctx context.Context, q querier, t customerRefreshToken, tokenHash string) error {
	_, err := q.Exec(ctx, `INSERT INTO customer_refresh_tokens
		(id, customer_id, token_hash, family_id, parent_token_id, remember_me,
		 created_at, expires_at, absolute_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.CustomerID, tokenHash, t.FamilyID, t.ParentID, t.RememberMe,
		t.CreatedAt, t.ExpiresAt, t.AbsoluteExpiresAt)
	return err
}

// findCustomerRefreshTokenByHash locks the row so concurrent refreshes of the same token are serialized. It
// returns (nil, nil) when no token matches.
func findCustomerRefreshTokenByHash(ctx context.Context, q querier, tokenHash string) (*customerRefreshToken, error) {
	var t customerRefreshToken
	err := q.QueryRow(ctx, `SELECT id, customer_id, family_id, parent_token_id, remember_me,
			created_at, expires_at, absolute_expires_at, rotated_at, revoked_at
		FROM customer_refresh_tokens WHERE token_hash = $1 FOR UPDATE`, tokenHash).
		Scan(&t.ID, &t.CustomerID, &t.FamilyID, &t.ParentID, &t.RememberMe,
			&t.CreatedAt, &t.ExpiresAt, &t.AbsoluteExpiresAt, &t.RotatedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func markCustomerRefreshRotated(ctx context.Context, q querier, id uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customer_refresh_tokens SET rotated_at = $2 WHERE id = $1`, id, now)
	return err
}

func revokeCustomerRefreshFamily(ctx context.Context, q querier, familyID uuid.UUID, reason string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customer_refresh_tokens
		SET revoked_at = $3, revoked_reason = $2::text::revoked_reason
		WHERE family_id = $1 AND revoked_at IS NULL`, familyID, reason, now)
	return err
}

func revokeAllCustomerRefreshTokens(ctx context.Context, q querier, customerID uuid.UUID, reason string, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customer_refresh_tokens
		SET revoked_at = $3, revoked_reason = $2::text::revoked_reason
		WHERE customer_id = $1 AND revoked_at IS NULL`, customerID, reason, now)
	return err
}

// latestCustomerPasswordResetQueuedAt tells when the last reset email was queued for a customer.
func latestCustomerPasswordResetQueuedAt(ctx context.Context, q querier, customerID uuid.UUID) (*time.Time, error) {
	var latest *time.Time
	err := q.QueryRow(ctx, `SELECT max(created_at) FROM notification_logs
		WHERE recipient_customer_id = $1 AND notification_type = 'customer_password_reset'`, customerID).Scan(&latest)
	return latest, err
}
