package sales

import (
	"context"
	"encoding/json"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type querier = tasks.Querier

// Every sale query selects from this FROM clause, with the sale as s, its advisor as e and the derived
// current task as ct.
const saleFrom = ` FROM sale_registers s
	JOIN employees e ON e.id = s.assigned_advisor_id
	JOIN v_sale_current_task ct ON ct.sale_id = s.id`

const saleColumns = `s.id, s.customer_id, s.customer_legal_name_snapshot, s.customer_doc_type_snapshot::text,
	s.customer_doc_number_snapshot, s.customer_type_snapshot::text, s.vehicle_model_id, s.vehicle_version_id,
	s.vehicle_color_id, s.vehicle_model_name_snapshot, s.vehicle_version_name_snapshot, s.vehicle_color_name_snapshot,
	s.vehicle_reference_price_snapshot::text, s.sale_temperature::text, s.overall_state::text, s.vehicle_series, s.vin,
	s.license_plate, s.assigned_advisor_id, e.first_name || ' ' || e.last_name, s.sale_notes,
	ct.current_task_number, ct.current_task_status::text,
	(SELECT count(*) FROM task_instances t WHERE t.sale_id = s.id AND t.status = 'completed'),
	s.completed_at, s.cancelled_at, s.cancelled_at_task_number, s.cancellation_category::text, s.cancellation_text,
	s.row_version, s.created_at, s.updated_at`

func scanSale(row pgx.Row) (*Sale, error) {
	var s Sale
	err := row.Scan(&s.ID, &s.CustomerID, &s.CustomerName, &s.CustomerDocumentType, &s.CustomerDocument, &s.CustomerType,
		&s.ModelID, &s.VersionID, &s.ColorID, &s.ModelName, &s.VersionName, &s.ColorName, &s.ReferencePrice,
		&s.Temperature, &s.State, &s.VehicleSeries, &s.Vin, &s.LicensePlate, &s.AdvisorID, &s.AdvisorName, &s.Notes,
		&s.CurrentTaskNumber, &s.CurrentTaskStatus, &s.CompletedTasks, &s.CompletedAt, &s.CancelledAt,
		&s.CancelledAtTask, &s.CancellationCategory, &s.CancellationText, &s.Version, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// findSale returns (nil, nil) when the sale does not exist. With forUpdate the sale row is locked.
func findSale(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*Sale, error) {
	sql := `SELECT ` + saleColumns + saleFrom + ` WHERE s.id = $1`
	if forUpdate {
		sql += ` FOR UPDATE OF s`
	}
	s, err := scanSale(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func milestonesOf(ctx context.Context, q querier, saleID uuid.UUID) ([]Milestone, error) {
	rows, err := q.Query(ctx, `SELECT m.milestone_number, d.name_es, m.state::text
		FROM v_sale_milestones m JOIN milestone_definitions d ON d.milestone_number = m.milestone_number
		WHERE m.sale_id = $1 ORDER BY m.milestone_number`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.Number, &m.Name, &m.State); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// listSales returns one page and the total. advisorID, when set, restricts the list to that advisor's own
// sales. Sort values come from an allowlist, never from input.
func listSales(ctx context.Context, q querier, f ListFilter, advisorID *uuid.UUID) ([]Sale, int, error) {
	var where []string
	var args []any
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if advisorID != nil {
		where = append(where, "s.assigned_advisor_id = "+add(*advisorID))
	}
	if f.Search != "" {
		term := escapeLike(strings.TrimSpace(f.Search))
		name, doc, series, vin, plate := add(term), add(term), add(term), add(term), add(term)
		where = append(where, fmt.Sprintf(
			`(f_search_key(s.customer_legal_name_snapshot) LIKE '%%' || f_search_key(%s) || '%%' ESCAPE '\'
			  OR s.customer_doc_number_snapshot LIKE upper(%s) || '%%' ESCAPE '\'
			  OR s.vehicle_series LIKE '%%' || upper(%s) || '%%' ESCAPE '\'
			  OR s.vin LIKE '%%' || upper(%s) || '%%' ESCAPE '\'
			  OR s.license_plate LIKE '%%' || upper(%s) || '%%' ESCAPE '\')`, name, doc, series, vin, plate))
	}
	if f.State != "" {
		where = append(where, "s.overall_state = "+add(f.State)+"::text::sale_state")
	}
	if f.CurrentTask != 0 {
		where = append(where, "ct.current_task_number = "+add(f.CurrentTask))
	}
	if f.Milestone != 0 {
		where = append(where, fmt.Sprintf(`EXISTS (SELECT 1 FROM v_sale_milestones ms WHERE ms.sale_id = s.id
			AND ms.milestone_number = %s AND ms.state = 'in_progress')`, add(f.Milestone)))
	}
	if f.AdvisorID != nil {
		where = append(where, "s.assigned_advisor_id = "+add(*f.AdvisorID))
	}
	if f.CustomerType != "" {
		where = append(where, "s.customer_type_snapshot = "+add(f.CustomerType)+"::text::customer_type")
	}
	if f.Temperature != "" {
		where = append(where, "s.sale_temperature = "+add(f.Temperature)+"::text::sale_temperature")
	}
	if f.ModelID != nil {
		where = append(where, "s.vehicle_model_id = "+add(*f.ModelID))
	}
	if f.VersionID != nil {
		where = append(where, "s.vehicle_version_id = "+add(*f.VersionID))
	}
	clause := "TRUE"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*)`+saleFrom+` WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := map[string]string{
		"createdAt": "s.created_at", "-createdAt": "s.created_at DESC",
		"customerName": "f_search_key(s.customer_legal_name_snapshot)", "-customerName": "f_search_key(s.customer_legal_name_snapshot) DESC",
	}[f.Sort]
	if order == "" {
		order = "s.created_at DESC"
	}
	limit, offset := add(f.PageSize), add((f.Page-1)*f.PageSize)
	rows, err := q.Query(ctx, `SELECT `+saleColumns+saleFrom+` WHERE `+clause+
		` ORDER BY `+order+`, s.id LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []Sale{}
	for rows.Next() {
		s, err := scanSale(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *s)
	}
	return items, total, rows.Err()
}

// customerSnapshot is what a sale keeps of its customer.
type customerSnapshot struct {
	Name, DocumentType, DocumentNumber, Type string
	Active                                   bool
}

// vehicleSnapshot is what a sale keeps of its vehicle.
type vehicleSnapshot struct {
	ModelID                            uuid.UUID
	ModelName, VersionName, ColorName  string
	ReferencePrice                     string
	VersionSelectable, ColorSelectable bool
	ColorBelongsToModel                bool
}

// readCustomerSnapshot locks the customer row against changes until the sale is committed. Taking the
// snapshot reads the customers table directly: it is the one place where the sale needs a consistent copy
// of another module's data.
func readCustomerSnapshot(ctx context.Context, q querier, id uuid.UUID) (*customerSnapshot, error) {
	var c customerSnapshot
	err := q.QueryRow(ctx, `SELECT display_name, document_type::text, document_number, customer_type::text, is_active
		FROM customers WHERE id = $1 FOR SHARE`, id).Scan(&c.Name, &c.DocumentType, &c.DocumentNumber, &c.Type, &c.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

// readVehicleSnapshot locks the version, color and model rows and reports whether they can be selected.
// It returns (nil, nil) when the version does not exist or the color does not exist.
func readVehicleSnapshot(ctx context.Context, q querier, versionID, colorID uuid.UUID) (*vehicleSnapshot, error) {
	var v vehicleSnapshot
	err := q.QueryRow(ctx, `SELECT m.id, m.model_name, ver.version_name, col.color_name, ver.reference_price::text,
			EXISTS (SELECT 1 FROM v_selectable_versions sv WHERE sv.version_id = ver.id),
			EXISTS (SELECT 1 FROM v_selectable_colors sc WHERE sc.color_id = col.id),
			col.model_id = ver.model_id
		FROM vehicle_versions ver
		JOIN vehicle_models m ON m.id = ver.model_id
		JOIN vehicle_colors col ON col.id = $2
		WHERE ver.id = $1
		FOR SHARE OF ver, m, col`, versionID, colorID).
		Scan(&v.ModelID, &v.ModelName, &v.VersionName, &v.ColorName, &v.ReferencePrice,
			&v.VersionSelectable, &v.ColorSelectable, &v.ColorBelongsToModel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}

// advisorIsActive reports whether the employee is an active Sales Advisor.
func advisorIsActive(ctx context.Context, q querier, id uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees WHERE id = $1 AND role = 'sales_advisor' AND is_active)`,
		id).Scan(&ok)
	return ok, err
}

func insertSale(ctx context.Context, q querier, id uuid.UUID, in CreateInput, c *customerSnapshot, v *vehicleSnapshot,
	advisor, actor uuid.UUID, notes *string, now time.Time,
) error {
	_, err := q.Exec(ctx, `INSERT INTO sale_registers
		(id, customer_id, customer_legal_name_snapshot, customer_doc_type_snapshot, customer_doc_number_snapshot,
		 customer_type_snapshot, vehicle_model_id, vehicle_version_id, vehicle_color_id, vehicle_model_name_snapshot,
		 vehicle_version_name_snapshot, vehicle_color_name_snapshot, vehicle_reference_price_snapshot, sale_temperature,
		 assigned_advisor_id, sale_notes, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4::text::customer_document_type, $5, $6::text::customer_type, $7, $8, $9, $10, $11, $12,
		        $13::text::numeric, $14::text::sale_temperature, $15, $16, $17, $18, $18)`,
		id, in.CustomerID, c.Name, c.DocumentType, c.DocumentNumber, c.Type, v.ModelID, in.VersionID, in.ColorID,
		v.ModelName, v.VersionName, v.ColorName, v.ReferencePrice, in.Temperature, advisor, notes, actor, now)
	return err
}

func setNotes(ctx context.Context, q querier, id uuid.UUID, version int, notes *string, actor uuid.UUID, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE sale_registers SET sale_notes = $3, updated_by = $4, updated_at = $5,
		row_version = row_version + 1 WHERE id = $1 AND row_version = $2`, id, version, notes, actor, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func setAdvisor(ctx context.Context, q querier, id, advisor, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE sale_registers SET assigned_advisor_id = $2, updated_by = $3, updated_at = $4,
		row_version = row_version + 1 WHERE id = $1`, id, advisor, actor, now)
	return err
}

func markCancelled(ctx context.Context, q querier, id uuid.UUID, taskNumber int, category string, text *string, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE sale_registers SET overall_state = 'cancelled', cancelled_at = $2, cancelled_by = $3,
		cancelled_at_task_number = $4, cancellation_category = $5::text::cancellation_category, cancellation_text = $6,
		updated_by = $3, updated_at = $2, row_version = row_version + 1 WHERE id = $1`,
		id, now, actor, taskNumber, category, text)
	return err
}

func markReopened(ctx context.Context, q querier, id, actor uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE sale_registers SET overall_state = 'active', cancelled_at = NULL, cancelled_by = NULL,
		cancelled_at_task_number = NULL, cancellation_category = NULL, cancellation_text = NULL,
		updated_by = $2, updated_at = $3, row_version = row_version + 1 WHERE id = $1`, id, actor, now)
	return err
}

// listHistory returns one page of the sale's audit timeline, newest first.
func listHistory(ctx context.Context, q querier, saleID uuid.UUID, page, pageSize int) ([]HistoryEntry, int, error) {
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE sale_id = $1`, saleID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT a.id, a.actor_employee_id, e.first_name || ' ' || e.last_name, a.actor_kind::text,
			a.entity_type::text, a.entity_id, a.action::text, a.changes_summary::text, a.reason, a.created_at
		FROM audit_logs a LEFT JOIN employees e ON e.id = a.actor_employee_id
		WHERE a.sale_id = $1 ORDER BY a.created_at DESC, a.id DESC LIMIT $2 OFFSET $3`,
		saleID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		var changes *string
		if err := rows.Scan(&h.ID, &h.ActorID, &h.ActorName, &h.ActorKind, &h.EntityType, &h.EntityID,
			&h.Action, &changes, &h.Reason, &h.CreatedAt); err != nil {
			return nil, 0, err
		}
		if changes != nil {
			h.Changes = json.RawMessage(*changes)
		}
		items = append(items, h)
	}
	return items, total, rows.Err()
}

// wasDelivered reports whether the sale ever completed Task 13: now, or at any earlier time (a reopened Task 13
// has no completion date left, but its completion is in the audit log).
func wasDelivered(ctx context.Context, q querier, saleID uuid.UUID) (bool, error) {
	var delivered bool
	err := q.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM task_instances WHERE sale_id = $1 AND task_number = 13 AND status = 'completed')
		OR EXISTS (SELECT 1 FROM audit_logs WHERE sale_id = $1 AND entity_type = 'task_instance'
		           AND action = 'COMPLETED' AND changes_summary ->> 'taskNumber' = '13')
		OR EXISTS (SELECT 1 FROM audit_logs WHERE sale_id = $1 AND entity_type = 'sale_register' AND action = 'COMPLETED')`,
		saleID).Scan(&delivered)
	return delivered, err
}

// deleteSale removes the sale. Its tasks, detail rows, payments, invoices and documents go with it (cascade).
func deleteSale(ctx context.Context, q querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM sale_registers WHERE id = $1`, id)
	return err
}
