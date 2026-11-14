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

// limaZone is fixed because Peru has no daylight saving time.
var limaZone = time.FixedZone("America/Lima", -5*60*60)

// saleCode is the human-friendly number: the year the sale was created (Lima time) and its sequence number.
func saleCode(createdAt time.Time, number int64) string {
	return fmt.Sprintf("EF-%d-%05d", createdAt.In(limaZone).Year(), number)
}

// saleRow is one sale as stored, joined with the people and the image the detail shows.
type saleRow struct {
	Sale
	number         int64
	cancelledAtNum *int
	advisorID      uuid.UUID
	imageKey       *string
}

const saleSelect = `SELECT s.id, s.sale_number, s.created_at, s.updated_at, s.completed_at, s.overall_state::text, s.sale_temperature::text, s.sale_notes, s.row_version,
		s.customer_id, s.customer_legal_name_snapshot, s.customer_doc_type_snapshot::text, s.customer_doc_number_snapshot, s.customer_type_snapshot::text,
		s.vehicle_model_id, s.vehicle_version_id, s.vehicle_color_id, s.vehicle_model_name_snapshot, s.vehicle_version_name_snapshot,
		s.vehicle_color_name_snapshot, s.vehicle_reference_price_snapshot::text, s.vehicle_series, s.vin, s.license_plate,
		s.assigned_advisor_id, a.first_name || ' ' || a.last_name, a.business_email, a.business_phone,
		s.cancelled_at, s.cancellation_category::text, s.cancellation_text, s.cancelled_at_task_number, coalesce(cb.first_name || ' ' || cb.last_name, ''),
		f.quoted_final_price::text,
		(SELECT i.storage_key FROM catalog_images i WHERE i.vehicle_color_id = s.vehicle_color_id AND i.viewpoint = 'main_view')
	FROM sale_registers s
	JOIN employees a ON a.id = s.assigned_advisor_id
	LEFT JOIN employees cb ON cb.id = s.cancelled_by
	LEFT JOIN sale_financials f ON f.sale_id = s.id`

func loadSaleRow(ctx context.Context, q querier, id uuid.UUID) (*saleRow, error) {
	var r saleRow
	var cancelledAt *time.Time
	var category, text *string
	var cancelledBy string
	err := q.QueryRow(ctx, saleSelect+` WHERE s.id = $1`, id).Scan(
		&r.ID, &r.number, &r.CreatedAt, &r.UpdatedAt, &r.CompletedAt, &r.State, &r.Temperature, &r.Notes, &r.Version,
		&r.Customer.ID, &r.Customer.Name, &r.Customer.DocumentType, &r.Customer.DocumentNumber, &r.Customer.CustomerType,
		&r.Vehicle.ModelID, &r.Vehicle.VersionID, &r.Vehicle.ColorID, &r.Vehicle.Model, &r.Vehicle.Version,
		&r.Vehicle.Color, &r.Vehicle.ReferencePrice, &r.Vehicle.Series, &r.Vehicle.VIN, &r.Vehicle.LicensePlate,
		&r.advisorID, &r.Advisor.Name, &r.Advisor.Email, &r.Advisor.Phone,
		&cancelledAt, &category, &text, &r.cancelledAtNum, &cancelledBy,
		&r.QuotedFinalPrice, &r.imageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Code = saleCode(r.CreatedAt, r.number)
	r.Advisor.ID = r.advisorID
	if cancelledAt != nil && category != nil && r.cancelledAtNum != nil {
		r.Cancellation = &Cancellation{
			Category: *category, Text: valueOr(text), CancelledAt: *cancelledAt,
			CancelledByName: cancelledBy, AtTaskNumber: *r.cancelledAtNum,
		}
	}
	return &r, nil
}

func valueOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type milestoneDef struct {
	number      int
	name        string
	description string
}

func loadMilestoneDefs(ctx context.Context, q querier) ([]milestoneDef, error) {
	rows, err := q.Query(ctx, `SELECT milestone_number, name_es, description_es FROM milestone_definitions ORDER BY milestone_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []milestoneDef
	for rows.Next() {
		var d milestoneDef
		if err := rows.Scan(&d.number, &d.name, &d.description); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// --- the workspace list ---

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

type listRow struct {
	ListItem
	number   int64
	imageKey *string
	taskNum  *int
	taskName *string
	taskStat *string
	taskDue  *time.Time
	taskMile *int
}

// listSales returns one page of sales and the total. advisorScope, when set, limits the list to that
// advisor's sales; the sort comes from an allowlist, never from input.
func listSales(ctx context.Context, q querier, f ListFilter, advisorScope *uuid.UUID, now time.Time) ([]listRow, int, error) {
	var where []string
	var args []any
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if advisorScope != nil {
		where = append(where, "s.assigned_advisor_id = "+add(*advisorScope))
	}
	if f.State != "" {
		where = append(where, "s.overall_state = "+add(f.State)+"::text::sale_state")
	}
	if f.CurrentTask > 0 {
		where = append(where, "ct.task_number = "+add(f.CurrentTask))
	}
	if f.Milestone > 0 {
		where = append(where, "td.milestone_number = "+add(f.Milestone))
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
	if q := strings.TrimSpace(f.Search); q != "" {
		term := escapeLike(q)
		a, b, c, d := add(term), add(term), add(term), add(term)
		where = append(where, fmt.Sprintf(`(f_search_key(s.customer_legal_name_snapshot) LIKE '%%' || f_search_key(%s) || '%%' ESCAPE '\'
			OR s.customer_doc_number_snapshot LIKE upper(%s) || '%%' ESCAPE '\'
			OR f_search_key(s.vehicle_model_name_snapshot || ' ' || s.vehicle_version_name_snapshot || ' ' || s.vehicle_color_name_snapshot) LIKE '%%' || f_search_key(%s) || '%%' ESCAPE '\'
			OR s.vehicle_series ILIKE '%%' || %s || '%%' ESCAPE '\' OR s.vin ILIKE '%%' || %s || '%%' ESCAPE '\' OR s.license_plate ILIKE '%%' || %s || '%%' ESCAPE '\'
			OR ('EF-' || to_char(s.created_at AT TIME ZONE INTERVAL '-05:00', 'YYYY') || '-' || lpad(s.sale_number::text, 5, '0')) ILIKE '%%' || %s || '%%' ESCAPE '\')`, a, b, c, d, add(term), add(term), add(term)))
	}
	clause := "TRUE"
	if len(where) > 0 {
		clause = strings.Join(where, " AND ")
	}
	from := `FROM sale_registers s
		JOIN employees a ON a.id = s.assigned_advisor_id
		LEFT JOIN LATERAL (
			SELECT t.task_number, t.status::text AS status, t.due_date FROM task_instances t
			WHERE t.sale_id = s.id AND t.status <> 'completed'
			ORDER BY (t.status = 'in_progress') DESC, t.task_number LIMIT 1) ct ON TRUE
		LEFT JOIN task_definitions td ON td.task_number = ct.task_number`

	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) `+from+` WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := map[string]string{
		"-createdAt": "s.created_at DESC", "createdAt": "s.created_at",
		"customerName": "lower(s.customer_legal_name_snapshot)", "-customerName": "lower(s.customer_legal_name_snapshot) DESC",
		"updatedAt": "s.updated_at", "-updatedAt": "s.updated_at DESC",
		"dueDate": "ct.due_date NULLS LAST",
	}[f.Sort]
	if order == "" {
		order = "s.created_at DESC"
	}
	tiebreak := "s.id"
	if strings.HasSuffix(order, "DESC") {
		tiebreak = "s.id DESC"
	}
	limit, offset := add(f.PageSize), add((f.Page-1)*f.PageSize)
	rows, err := q.Query(ctx, `SELECT s.id, s.sale_number, s.created_at, s.updated_at, s.customer_legal_name_snapshot, s.customer_doc_type_snapshot::text || ' ' || s.customer_doc_number_snapshot,
			s.customer_type_snapshot::text, s.vehicle_model_name_snapshot, s.vehicle_version_name_snapshot, s.vehicle_color_name_snapshot,
			s.assigned_advisor_id, a.first_name || ' ' || a.last_name, s.sale_temperature::text, s.overall_state::text,
			ct.task_number, td.name_es, ct.status, ct.due_date, td.milestone_number,
			(SELECT i.storage_key FROM catalog_images i WHERE i.vehicle_color_id = s.vehicle_color_id AND i.viewpoint = 'main_view')
		`+from+` WHERE `+clause+` ORDER BY `+order+`, `+tiebreak+` LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []listRow
	for rows.Next() {
		var r listRow
		if err := rows.Scan(&r.ID, &r.number, &r.CreatedAt, &r.UpdatedAt, &r.CustomerName, &r.CustomerDocument, &r.CustomerType,
			&r.VehicleModel, &r.VehicleVersion, &r.VehicleColor, &r.AdvisorID, &r.AdvisorName, &r.Temperature, &r.State,
			&r.taskNum, &r.taskName, &r.taskStat, &r.taskDue, &r.taskMile, &r.imageKey); err != nil {
			return nil, 0, err
		}
		r.Code = saleCode(r.CreatedAt, r.number)
		if r.taskNum != nil {
			ct := &CurrentTask{Number: *r.taskNum, Name: valueOr(r.taskName), Status: tasks.Status(valueOr(r.taskStat)), DueDate: r.taskDue}
			if r.taskMile != nil {
				ct.Milestone = *r.taskMile
			}
			ct.IsOverdue = ct.Status == tasks.StatusInProgress && r.taskDue != nil && r.taskDue.Before(now)
			r.CurrentTask = ct
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// --- creating ---

type customerRow struct {
	ID           uuid.UUID
	Name         string
	DocType      string
	DocNumber    string
	CustomerType string
	Active       bool
}

func loadCustomer(ctx context.Context, q querier, id uuid.UUID) (*customerRow, error) {
	var c customerRow
	err := q.QueryRow(ctx, `SELECT id, display_name, document_type::text, document_number, customer_type::text, is_active
		FROM customers WHERE id = $1 FOR SHARE`, id).
		Scan(&c.ID, &c.Name, &c.DocType, &c.DocNumber, &c.CustomerType, &c.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

type vehicleRow struct {
	ModelID      uuid.UUID
	ModelName    string
	VersionName  string
	ColorName    string
	Price        string
	Selectable   bool
	ColorOK      bool
	ColorModelID uuid.UUID
	ColorFound   bool
	VersionFound bool
}

// loadVehicle reads the chosen version and color and whether the catalog offers them (the v_selectable_*
// views hold the rule: entry and model active, and the color has its Main view image).
func loadVehicle(ctx context.Context, q querier, versionID, colorID uuid.UUID) (*vehicleRow, error) {
	v := &vehicleRow{}
	err := q.QueryRow(ctx, `SELECT m.id, m.model_name, v.version_name, v.reference_price::text, (sv.version_id IS NOT NULL)
		FROM vehicle_versions v JOIN vehicle_models m ON m.id = v.model_id
		LEFT JOIN v_selectable_versions sv ON sv.version_id = v.id WHERE v.id = $1 FOR SHARE OF v, m`, versionID).Scan(&v.ModelID, &v.ModelName, &v.VersionName, &v.Price, &v.Selectable)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	v.VersionFound = true
	err = q.QueryRow(ctx, `SELECT c.model_id, c.color_name, (sc.color_id IS NOT NULL)
		FROM vehicle_colors c LEFT JOIN v_selectable_colors sc ON sc.color_id = c.id WHERE c.id = $1 FOR SHARE OF c`, colorID).Scan(&v.ColorModelID, &v.ColorName, &v.ColorOK)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	v.ColorFound = err == nil
	return v, err
}

type advisorRow struct {
	ID     uuid.UUID
	Name   string
	Email  string
	Role   string
	Active bool
}

func loadAdvisor(ctx context.Context, q querier, id uuid.UUID) (*advisorRow, error) {
	var a advisorRow
	err := q.QueryRow(ctx, `SELECT id, first_name || ' ' || last_name, business_email, role::text, is_active FROM employees WHERE id = $1`, id).
		Scan(&a.ID, &a.Name, &a.Email, &a.Role, &a.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

// --- history ---

func loadHistory(ctx context.Context, q querier, saleID uuid.UUID, page, pageSize int) ([]HistoryEvent, int, error) {
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE sale_id = $1`, saleID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT l.id, l.created_at, coalesce(e.first_name || ' ' || e.last_name, ''), l.actor_employee_id IS NULL, l.entity_type::text,
			t.task_number, l.action::text, l.reason, l.changes_summary
		FROM audit_logs l LEFT JOIN employees e ON e.id = l.actor_employee_id
		LEFT JOIN task_instances t ON t.id = l.entity_id AND l.entity_type = 'task_instance'
		WHERE l.sale_id = $1 ORDER BY l.created_at DESC, l.id DESC LIMIT $2 OFFSET $3`, saleID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []HistoryEvent{}
	for rows.Next() {
		var h HistoryEvent
		var raw []byte
		if err := rows.Scan(&h.ID, &h.At, &h.ActorName, &h.IsSystem, &h.EntityType, &h.TaskNumber, &h.Action, &h.Reason, &raw); err != nil {
			return nil, 0, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &h.Changes)
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}
