package staff

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

const employeeColumns = `id, document_type::text, document_number, first_name, last_name, name_source::text,
	nationality, gender::text, birthdate, hire_date, personal_phone, personal_email,
	business_phone, business_email, role::text, is_active, profile_picture_key IS NOT NULL, activated_at, row_version, created_at, updated_at`

func scanEmployee(row pgx.Row) (*Employee, error) {
	var e Employee
	var birthdate, hireDate time.Time
	err := row.Scan(&e.ID, &e.DocumentType, &e.DocumentNumber, &e.FirstName, &e.LastName, &e.NameSource,
		&e.Nationality, &e.Gender, &birthdate, &hireDate, &e.PersonalPhone, &e.PersonalEmail,
		&e.BusinessPhone, &e.BusinessEmail, &e.Role, &e.IsActive, &e.HasPhoto, &e.ActivatedAt, &e.Version,
		&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	e.Birthdate = birthdate.Format("2006-01-02")
	e.HireDate = hireDate.Format("2006-01-02")
	return &e, nil
}

// findEmployee returns (nil, nil) when the employee does not exist. The system actor is never returned.
func findEmployee(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*Employee, error) {
	sql := `SELECT ` + employeeColumns + ` FROM employees WHERE id = $1 AND NOT is_system`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	e, err := scanEmployee(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func insertEmployee(ctx context.Context, q querier, id uuid.UUID, d employeeData, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, name_source, nationality, gender,
		 birthdate, hire_date, personal_phone, personal_email, business_phone, business_email, role,
		 created_by, created_at, updated_at)
		VALUES ($1, $2::text::employee_document_type, $3, $4, $5, $6::text::name_source, $7,
				$8::text::gender, $9, $10, $11, $12, $13, $14, $15::text::employee_role, $16, $17, $17)`,
		id, d.DocumentType, d.DocumentNumber, d.FirstName, d.LastName, d.NameSource, d.Nationality,
		d.Gender, d.Birthdate, d.HireDate, d.PersonalPhone, d.PersonalEmail, d.BusinessPhone,
		d.BusinessEmail, d.Role, actor, now)
	return err
}

// updateEmployee writes every editable column and bumps the version, only if the version still matches.
func updateEmployee(ctx context.Context, q querier, id uuid.UUID, version int, d employeeData, actor uuid.UUID, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE employees SET
			document_type = $3::text::employee_document_type, document_number = $4,
			first_name = $5, last_name = $6, name_source = $7::text::name_source, nationality = $8,
			gender = $9::text::gender, birthdate = $10, hire_date = $11,
			personal_phone = $12, personal_email = $13, business_phone = $14, business_email = $15,
			role = $16::text::employee_role,
			updated_by = $17, updated_at = $18, row_version = row_version + 1
		WHERE id = $1 AND row_version = $2 AND NOT is_system`,
		id, version, d.DocumentType, d.DocumentNumber, d.FirstName, d.LastName, d.NameSource, d.Nationality,
		d.Gender, d.Birthdate, d.HireDate, d.PersonalPhone, d.PersonalEmail, d.BusinessPhone, d.BusinessEmail,
		d.Role, actor, now)
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
	_, err := q.Exec(ctx, `UPDATE employees SET is_active = $2, deactivated_at = $3, deactivated_by = $4,
			updated_by = $5, updated_at = $6, row_version = row_version + 1
		WHERE id = $1 AND NOT is_system`, id, active, deactivatedAt, deactivatedBy, actor, now)
	return err
}

func countActiveSales(ctx context.Context, q querier, advisorID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM sale_registers
		WHERE assigned_advisor_id = $1 AND overall_state = 'active'`, advisorID).Scan(&n)
	return n, err
}

// hasLoggedHistory reports whether the employee ever acted or signed in. Those records have no foreign key,
// so they are checked explicitly; foreign keys protect the rest.
func hasLoggedHistory(ctx context.Context, q querier, id uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_logs WHERE actor_employee_id = $1)
		OR EXISTS (SELECT 1 FROM access_logs WHERE employee_id = $1)`, id).Scan(&exists)
	return exists, err
}

func deleteEmployee(ctx context.Context, q querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM employees WHERE id = $1 AND NOT is_system`, id)
	return err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// listEmployees returns one page and the total count. Sort values come from an allowlist, never from input.
func listEmployees(ctx context.Context, q querier, f ListFilter) ([]Employee, int, error) {
	where := []string{"NOT is_system"}
	var args []any
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.Search != "" {
		term := escapeLike(strings.TrimSpace(f.Search))
		name, doc, mail := add(term), add(term), add(term)
		where = append(where, fmt.Sprintf(
			`(f_search_key(first_name || ' ' || last_name) LIKE '%%' || f_search_key(%s) || '%%' ESCAPE '\'
			  OR document_number LIKE upper(%s) || '%%' ESCAPE '\'
			  OR business_email LIKE lower(%s) || '%%' ESCAPE '\')`, name, doc, mail))
	}
	if f.Role != "" {
		where = append(where, "role = "+add(f.Role)+"::text::employee_role")
	}
	switch f.Status {
	case "active":
		where = append(where, "is_active")
	case "inactive":
		where = append(where, "NOT is_active")
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM employees WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := map[string]string{
		"lastName": "last_name, first_name", "-lastName": "last_name DESC, first_name DESC",
		"createdAt": "created_at", "-createdAt": "created_at DESC",
	}[f.Sort]
	if order == "" {
		order = "last_name, first_name"
	}
	limit, offset := add(f.PageSize), add((f.Page-1)*f.PageSize)
	rows, err := q.Query(ctx, `SELECT `+employeeColumns+` FROM employees WHERE `+clause+
		` ORDER BY `+order+`, id LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []Employee{}
	for rows.Next() {
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *e)
	}
	return items, total, rows.Err()
}

func listAdvisors(ctx context.Context, q querier) ([]Advisor, error) {
	rows, err := q.Query(ctx, `SELECT id, first_name, last_name FROM employees
		WHERE role = 'sales_advisor' AND is_active AND NOT is_system ORDER BY last_name, first_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	advisors := []Advisor{}
	for rows.Next() {
		var a Advisor
		if err := rows.Scan(&a.ID, &a.FirstName, &a.LastName); err != nil {
			return nil, err
		}
		advisors = append(advisors, a)
	}
	return advisors, rows.Err()
}
