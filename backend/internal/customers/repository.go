package customers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// querier is satisfied by the pool and by a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Every query aliases the customers table as c.
const customerColumns = `c.id, c.customer_type::text, c.document_type::text, c.document_number,
	c.first_name, c.last_name, c.gender::text, c.legal_name, c.display_name, c.name_source::text,
	c.nationality, c.phone, c.email, c.contact_first_name, c.contact_last_name, c.contact_job_title,
	c.activated_at, c.is_active, c.row_version, c.created_at, c.updated_at`

func scanCustomer(row pgx.Row) (*Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.CustomerType, &c.DocumentType, &c.DocumentNumber,
		&c.FirstName, &c.LastName, &c.Gender, &c.LegalName, &c.DisplayName, &c.NameSource,
		&c.Nationality, &c.Phone, &c.Email, &c.ContactFirstName, &c.ContactLastName, &c.ContactJobTitle,
		&c.ActivatedAt, &c.IsActive, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// findCustomer returns (nil, nil) when the customer does not exist.
func findCustomer(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*Customer, error) {
	sql := `SELECT ` + customerColumns + ` FROM customers c WHERE c.id = $1`
	if forUpdate {
		sql += ` FOR UPDATE OF c`
	}
	c, err := scanCustomer(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// findByDocument returns (nil, nil) when no customer has that document.
func findByDocument(ctx context.Context, q querier, docType, docNumber string) (*Customer, error) {
	c, err := scanCustomer(q.QueryRow(ctx, `SELECT `+customerColumns+` FROM customers c
		WHERE c.document_type = $1::text::customer_document_type AND c.document_number = $2`, docType, docNumber))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// advisorScopeSQL is the visibility rule for Advisors (FR-026): customers with at least one sale currently
// assigned to them, plus customers they created that have no sale yet. %[1]s is the placeholder holding
// the advisor's id.
const advisorScopeSQL = `(EXISTS (SELECT 1 FROM sale_registers s WHERE s.customer_id = c.id AND s.assigned_advisor_id = %[1]s)
	OR (c.created_by = %[1]s AND NOT EXISTS (SELECT 1 FROM sale_registers s WHERE s.customer_id = c.id)))`

func inAdvisorScope(ctx context.Context, q querier, customerID, advisorID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM customers c WHERE c.id = $1 AND `+
		fmt.Sprintf(advisorScopeSQL, "$2")+`)`, customerID, advisorID).Scan(&ok)
	return ok, err
}

func insertCustomer(ctx context.Context, q querier, id uuid.UUID, d customerData, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO customers
		(id, customer_type, document_type, document_number, first_name, last_name, gender, legal_name,
		 name_source, nationality, phone, email, contact_first_name, contact_last_name, contact_job_title,
		 created_by, created_at, updated_at)
		VALUES ($1, $2::text::customer_type, $3::text::customer_document_type, $4, $5, $6, $7::text::gender, $8,
				$9::text::name_source, $10, $11, $12, $13, $14, $15, $16, $17, $17)`,
		id, d.CustomerType, d.DocumentType, d.DocumentNumber, d.FirstName, d.LastName, d.Gender, d.LegalName,
		d.NameSource, d.Nationality, d.Phone, d.Email, d.ContactFirstName, d.ContactLastName, d.ContactJobTitle,
		actor, now)
	return err
}

// updateCustomer writes every editable column and bumps the version, only if the version still matches.
func updateCustomer(ctx context.Context, q querier, id uuid.UUID, version int, d customerData, actor uuid.UUID, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE customers SET
			customer_type = $3::text::customer_type, document_type = $4::text::customer_document_type,
			document_number = $5, first_name = $6, last_name = $7, gender = $8::text::gender, legal_name = $9,
			name_source = $10::text::name_source, nationality = $11, phone = $12, email = $13,
			contact_first_name = $14, contact_last_name = $15, contact_job_title = $16,
			updated_by = $17, updated_at = $18, row_version = row_version + 1
		WHERE id = $1 AND row_version = $2`,
		id, version, d.CustomerType, d.DocumentType, d.DocumentNumber, d.FirstName, d.LastName, d.Gender,
		d.LegalName, d.NameSource, d.Nationality, d.Phone, d.Email, d.ContactFirstName, d.ContactLastName,
		d.ContactJobTitle, actor, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func setActive(ctx context.Context, q querier, id uuid.UUID, active bool, actor uuid.UUID, now time.Time) error {
	var deactivatedAt *time.Time
	var deactivatedBy *uuid.UUID
	if !active {
		deactivatedAt, deactivatedBy = &now, &actor
	}
	_, err := q.Exec(ctx, `UPDATE customers SET is_active = $2, deactivated_at = $3, deactivated_by = $4,
			updated_by = $5, updated_at = $6, row_version = row_version + 1
		WHERE id = $1`, id, active, deactivatedAt, deactivatedBy, actor, now)
	return err
}

func countSales(ctx context.Context, q querier, customerID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM sale_registers WHERE customer_id = $1`, customerID).Scan(&n)
	return n, err
}

// deleteCustomer removes a customer together with the welcome email and activation links created for them,
// which would otherwise keep the row referenced.
func deleteCustomer(ctx context.Context, q querier, id uuid.UUID) error {
	if _, err := q.Exec(ctx, `DELETE FROM customer_refresh_tokens WHERE customer_id = $1`, id); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM account_tokens WHERE customer_id = $1`, id); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM notification_logs WHERE recipient_customer_id = $1
		  AND notification_type IN ('customer_activation', 'customer_activated', 'customer_password_reset')`, id); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `DELETE FROM customers WHERE id = $1`, id)
	return err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// listCustomers returns one page and the total count. advisorID, when set, restricts the list to that
// advisor's scope. Sort values come from an allowlist, never from input.
func listCustomers(ctx context.Context, q querier, f ListFilter, advisorID *uuid.UUID) ([]Customer, int, error) {
	var where []string
	var args []any
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if advisorID != nil {
		where = append(where, fmt.Sprintf(advisorScopeSQL, add(*advisorID)))
	}
	if f.Search != "" {
		term := escapeLike(strings.TrimSpace(f.Search))
		name, doc, mail := add(term), add(term), add(term)
		where = append(where, fmt.Sprintf(
			`(f_search_key(c.display_name) LIKE '%%' || f_search_key(%s) || '%%' ESCAPE '\'
			  OR c.document_number LIKE upper(%s) || '%%' ESCAPE '\'
			  OR c.email LIKE lower(%s) || '%%' ESCAPE '\')`, name, doc, mail))
	}
	if f.CustomerType != "" {
		where = append(where, "c.customer_type = "+add(f.CustomerType)+"::text::customer_type")
	}
	switch f.Status {
	case "active":
		where = append(where, "c.is_active")
	case "inactive":
		where = append(where, "NOT c.is_active")
	}
	clause := "TRUE"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM customers c WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := map[string]string{
		"displayName": "c.display_name", "-displayName": "c.display_name DESC",
		"createdAt": "c.created_at", "-createdAt": "c.created_at DESC",
	}[f.Sort]
	if order == "" {
		order = "c.display_name"
	}
	limit, offset := add(f.PageSize), add((f.Page-1)*f.PageSize)
	rows, err := q.Query(ctx, `SELECT `+customerColumns+` FROM customers c WHERE `+clause+
		` ORDER BY `+order+`, c.id LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *c)
	}
	return items, total, rows.Err()
}

// --- lookup log (rate limiting and the monthly Codart counter) ---

func insertLookupLog(ctx context.Context, q querier, id, employeeID uuid.UUID, kind, docType, docNumber, outcome string, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO lookup_logs (id, employee_id, lookup_kind, document_type, document_number, outcome, created_at)
		VALUES ($1, $2, $3::text::lookup_kind, $4, $5, $6::text::lookup_outcome, $7)`,
		id, employeeID, kind, docType, docNumber, outcome, now)
	return err
}

func setLookupOutcome(ctx context.Context, q querier, id uuid.UUID, outcome string) error {
	_, err := q.Exec(ctx, `UPDATE lookup_logs SET outcome = $2::text::lookup_outcome WHERE id = $1`, id, outcome)
	return err
}

// countRecentLookups counts an employee's lookups of the given kinds since a moment, ignoring requests
// that were themselves refused (so a refusal does not extend the lockout).
func countRecentLookups(ctx context.Context, q querier, employeeID uuid.UUID, kinds []string, since time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM lookup_logs
		WHERE employee_id = $1 AND lookup_kind::text = ANY($2) AND created_at >= $3
		  AND outcome NOT IN ('rate_limited', 'quota_blocked')`, employeeID, kinds, since).Scan(&n)
	return n, err
}

// countProviderCalls counts the requests that reached Codart in a period. A call that failed is counted:
// the provider may have charged it, and being conservative protects the monthly allowance.
func countProviderCalls(ctx context.Context, q querier, from, to time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM lookup_logs
		WHERE lookup_kind IN ('codart_dni', 'codart_ruc') AND created_at >= $1 AND created_at < $2
		  AND outcome IN ('success', 'not_found', 'error')`, from, to).Scan(&n)
	return n, err
}
