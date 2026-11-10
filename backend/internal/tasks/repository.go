package tasks

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

type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Querier is the subset of database access the engine needs, exported so the sales package can share one
// transaction with it.
type Querier = querier

// --- definitions ---

type definition struct {
	name          string
	milestone     int
	durationHours *int
	reminderHours *int
}

func loadDefinitions(ctx context.Context, q querier) (map[int]definition, error) {
	rows, err := q.Query(ctx, `SELECT task_number, name_es, milestone_number, recommended_duration_hours, reminder_interval_hours FROM task_definitions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]definition{}
	for rows.Next() {
		var n int
		var d definition
		if err := rows.Scan(&n, &d.name, &d.milestone, &d.durationHours, &d.reminderHours); err != nil {
			return nil, err
		}
		out[n] = d
	}
	return out, rows.Err()
}

// --- task rows ---

type taskRow struct {
	ID              uuid.UUID
	Number          int
	Status          Status
	StartedAt       *time.Time
	CompletedAt     *time.Time
	DueDate         *time.Time
	CompletionPhase *Phase
	Notes           *string
	Version         int
}

func loadTaskRows(ctx context.Context, q querier, saleID uuid.UUID) ([]taskRow, error) {
	rows, err := q.Query(ctx, `SELECT id, task_number, status::text, started_at, completed_at, due_date, completion_phase::text, notes, row_version
		FROM task_instances WHERE sale_id = $1 ORDER BY task_number`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []taskRow
	for rows.Next() {
		var t taskRow
		var status string
		var phase *string
		if err := rows.Scan(&t.ID, &t.Number, &status, &t.StartedAt, &t.CompletedAt, &t.DueDate, &phase, &t.Notes, &t.Version); err != nil {
			return nil, err
		}
		t.Status = Status(status)
		if phase != nil {
			p := Phase(*phase)
			t.CompletionPhase = &p
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func statusesOf(rows []taskRow) []Status {
	out := make([]Status, len(rows))
	for i, r := range rows {
		out[i] = r.Status
	}
	return out
}

// --- financial snapshot, in cents ---

type fin struct {
	initialToyota     *int64
	initialCommission *int64
	accessory         *int64
	release           *int64
	quoted            *int64
	finalToyota       *int64
	finalCommission   *int64
}

func loadFin(ctx context.Context, q querier, saleID uuid.UUID) (fin, error) {
	var f fin
	err := q.QueryRow(ctx, `SELECT (initial_toyota_amount * 100)::bigint, (initial_commission * 100)::bigint, (accessory_amount * 100)::bigint,
			(release_processing_amount * 100)::bigint, (quoted_final_price * 100)::bigint, (final_toyota_amount * 100)::bigint,
			(final_commission * 100)::bigint
		FROM sale_financials WHERE sale_id = $1`, saleID).Scan(&f.initialToyota, &f.initialCommission, &f.accessory, &f.release,
		&f.quoted, &f.finalToyota, &f.finalCommission)
	return f, err
}

func (f fin) snapshot() financialSnapshot {
	return financialSnapshot{initialToyota: f.initialToyota, initialCommission: f.initialCommission}
}

// --- task data ---

func loadData(ctx context.Context, q querier, sale *SaleRef, n int) (any, error) {
	saleID := sale.ID
	switch n {
	case 1:
		var d Task1Data
		err := q.QueryRow(ctx, `SELECT initial_toyota_amount::text, initial_commission::text, accessory_amount::text, release_processing_amount::text
			FROM sale_financials WHERE sale_id = $1`, saleID).Scan(&d.InitialToyotaAmount, &d.InitialCommission, &d.AccessoryAmount, &d.ReleaseProcessingAmount)
		return &d, err
	case 2:
		var d Task2Data
		err := q.QueryRow(ctx, `SELECT signer_document_type::text, signer_document_number, signer_first_name, signer_last_name,
				signer_name_source::text, pep_status::text, ddc_regime::text FROM sale_due_diligence WHERE sale_id = $1`, saleID).Scan(
			&d.SignerDocumentType, &d.SignerDocumentNumber, &d.SignerFirstName, &d.SignerLastName, &d.SignerNameSource, &d.PepStatus, &d.DDCRegime)
		return &d, err
	case 3:
		d := Task3Data{VehicleSeries: sale.Series, VIN: sale.VIN}
		err := q.QueryRow(ctx, `SELECT final_toyota_amount::text FROM sale_financials WHERE sale_id = $1`, saleID).Scan(&d.FinalToyotaAmount)
		return &d, err
	case 4, 5:
		kind := "pana"
		if n == 5 {
			kind = "toyota"
		}
		var d PaymentData
		err := q.QueryRow(ctx, `SELECT bank::text, payment_date::text, transaction_number, deposited_amount::text, mismatch_reason
			FROM sale_payments WHERE sale_id = $1 AND payment_kind = $2::text::payment_kind`, saleID, kind).Scan(
			&d.Bank, &d.PaymentDate, &d.TransactionNumber, &d.DepositedAmount, &d.MismatchReason)
		if errors.Is(err, pgx.ErrNoRows) {
			return &d, nil
		}
		return &d, err
	case 6:
		d := Task6Data{Toyota: &InvoiceData{}, Commission: &InvoiceData{}, Accessories: &InvoiceData{}, ReleaseProcessing: &InvoiceData{}}
		rows, err := q.Query(ctx, `SELECT invoice_kind::text, invoice_number, invoice_date::text, invoice_amount::text, mismatch_reason
			FROM sale_invoices WHERE sale_id = $1`, saleID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var kind string
			var inv InvoiceData
			if err := rows.Scan(&kind, &inv.Number, &inv.Date, &inv.Amount, &inv.MismatchReason); err != nil {
				return nil, err
			}
			switch kind {
			case "toyota":
				d.Toyota = &inv
			case "commission":
				d.Commission = &inv
			case "accessories":
				d.Accessories = &inv
			case "release_processing":
				d.ReleaseProcessing = &inv
			}
		}
		return &d, rows.Err()
	case 7:
		return &Task7Data{}, nil
	case 8:
		var d Task8Data
		err := q.QueryRow(ctx, `SELECT dam_number, customs_office::text, control_channel::text FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(
			&d.DAMNumber, &d.CustomsOffice, &d.ControlChannel)
		return &d, err
	case 9:
		var d Task9Data
		err := q.QueryRow(ctx, `SELECT release_person_in_charge FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(&d.PersonInCharge)
		return &d, err
	case 10:
		d := Task10Data{LicensePlate: sale.Plate}
		err := q.QueryRow(ctx, `SELECT registration_date::text FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(&d.RegistrationDate)
		return &d, err
	case 11:
		var d Task11Data
		err := q.QueryRow(ctx, `SELECT pds_transfer_date::text FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(&d.PDSTransferDate)
		return &d, err
	case 12:
		var d Task12Data
		err := q.QueryRow(ctx, `SELECT equipment_service_order_number, equipment_completion_date::text FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(
			&d.ServiceOrderNumber, &d.EquipmentCompletionDate)
		return &d, err
	case 13:
		var d Task13Data
		err := q.QueryRow(ctx, `SELECT surquillo_transfer_date::text, vehicle_delivery_date::text FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(
			&d.SurquilloTransferDate, &d.DeliveryDate)
		return &d, err
	}
	return nil, fmt.Errorf("task %d does not exist", n)
}

// loadDates returns every date saved on the sale, with the group it belongs to, for the chronology warnings.
func loadDates(ctx context.Context, q querier, saleID uuid.UUID, createdAt time.Time) ([]DateEntry, error) {
	entries := []DateEntry{{Field: "saleCreated", Group: GroupSaleCreated, Date: dateOnly(createdAt)}}
	add := func(field string, group int, d *time.Time) {
		if d != nil {
			entries = append(entries, DateEntry{Field: field, Group: group, Date: *d})
		}
	}
	var reg, pds, equip, surq, deliv *time.Time
	if err := q.QueryRow(ctx, `SELECT registration_date, pds_transfer_date, equipment_completion_date, surquillo_transfer_date, vehicle_delivery_date
		FROM sale_fulfillment WHERE sale_id = $1`, saleID).Scan(&reg, &pds, &equip, &surq, &deliv); err != nil {
		return nil, err
	}
	add("registrationDate", GroupRegistration, reg)
	add("pdsTransferDate", GroupPDSTransfer, pds)
	add("equipmentCompletionDate", GroupEquipment, equip)
	add("surquilloTransferDate", GroupSurquillo, surq)
	add("deliveryDate", GroupDelivery, deliv)

	rows, err := q.Query(ctx, `SELECT payment_kind::text, payment_date FROM sale_payments WHERE sale_id = $1 AND payment_date IS NOT NULL`, saleID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind string
		var d time.Time
		if err := rows.Scan(&kind, &d); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "pana" {
			entries = append(entries, DateEntry{Field: "paymentDate", Group: GroupPanaPayment, Date: d})
		} else {
			entries = append(entries, DateEntry{Field: "paymentDate#toyota", Group: GroupToyotaPayment, Date: d})
		}
	}
	rows.Close()
	irows, err := q.Query(ctx, `SELECT invoice_kind::text, invoice_date FROM sale_invoices WHERE sale_id = $1 AND invoice_date IS NOT NULL`, saleID)
	if err != nil {
		return nil, err
	}
	defer irows.Close()
	for irows.Next() {
		var kind string
		var d time.Time
		if err := irows.Scan(&kind, &d); err != nil {
			return nil, err
		}
		entries = append(entries, DateEntry{Field: invoiceField(kind) + ".date", Group: GroupInvoices, Date: d})
	}
	return entries, irows.Err()
}

func invoiceField(kind string) string {
	if kind == "release_processing" {
		return "releaseProcessing"
	}
	return kind
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.In(limaZone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// limaZone is fixed because Peru has no daylight saving time.
var limaZone = time.FixedZone("America/Lima", -5*60*60)

// identifierOf maps the sale column a plan names to the identifier the sale gate understands.
func identifierOf(col string) Identifier {
	switch col {
	case "vehicle_series":
		return IdentSeries
	case "vin":
		return IdentVIN
	}
	return IdentPlate
}

// --- applying writes ---

// applyWrites runs the planned writes. Updates touch the one row of a sale-level table; per-kind tables are
// created on first save.
func applyWrites(ctx context.Context, q querier, gate SaleGate, saleID, actor uuid.UUID, writes []write, now time.Time) error {
	for _, w := range writes {
		if w.table == "sale_registers" {
			values := map[Identifier]*string{}
			for _, c := range w.cols {
				var v *string
				if str, ok := c.val.(string); ok {
					v = &str
				}
				values[identifierOf(c.col)] = v
			}
			if err := gate.SetIdentifiers(ctx, q, saleID, values, actor, now); err != nil {
				return err
			}
			continue
		}
		args := []any{saleID}
		param := func(v any, cast string) string {
			args = append(args, v)
			n := len(args)
			switch cast {
			case "":
				return fmt.Sprintf("$%d", n)
			case "numeric", "date":
				return fmt.Sprintf("$%d::%s", n, cast)
			default:
				return fmt.Sprintf("$%d::text::%s", n, cast)
			}
		}
		if w.kindCol == "" {
			sets := make([]string, 0, len(w.cols)+3)
			for _, c := range w.cols {
				sets = append(sets, c.col+" = "+param(c.val, c.cast))
			}
			sets = append(sets, "updated_by = "+param(actor, ""), "updated_at = "+param(now, ""), "row_version = row_version + 1")
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $1", w.table, strings.Join(sets, ", "), w.keyCol)
			if _, err := q.Exec(ctx, sql, args...); err != nil {
				return err
			}
			continue
		}
		// per-kind rows: insert or update
		cols := []string{"sale_id", w.kindCol}
		vals := []string{"$1", param(w.kindVal, w.kindCol[:strings.Index(w.kindCol, "_kind")]+"_kind")}
		sets := make([]string, 0, len(w.cols)+3)
		for _, c := range w.cols {
			cols = append(cols, c.col)
			ph := param(c.val, c.cast)
			vals = append(vals, ph)
			sets = append(sets, c.col+" = EXCLUDED."+c.col)
		}
		cols = append(cols, "updated_by", "updated_at")
		vals = append(vals, param(actor, ""), param(now, ""))
		sets = append(sets, "updated_by = EXCLUDED.updated_by", "updated_at = EXCLUDED.updated_at", fmt.Sprintf("row_version = %s.row_version + 1", w.table))
		sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (sale_id, %s) DO UPDATE SET %s",
			w.table, strings.Join(cols, ", "), strings.Join(vals, ", "), w.kindCol, strings.Join(sets, ", "))
		if _, err := q.Exec(ctx, sql, args...); err != nil {
			return err
		}
	}
	return nil
}

// clearExpected removes the stored expected amounts of a task's payments and invoices when the task goes back
// to being edited: the database refuses a differing amount without a reason while an expected amount is
// stored, and drafts must be savable.
func clearExpected(ctx context.Context, q querier, saleID uuid.UUID, n int) error {
	switch n {
	case 4, 5:
		kind := "pana"
		if n == 5 {
			kind = "toyota"
		}
		_, err := q.Exec(ctx, `UPDATE sale_payments SET expected_amount = NULL WHERE sale_id = $1 AND payment_kind = $2::text::payment_kind`, saleID, kind)
		return err
	case 6:
		_, err := q.Exec(ctx, `UPDATE sale_invoices SET expected_amount = NULL WHERE sale_id = $1`, saleID)
		return err
	}
	return nil
}

// CreateInitial writes the 13 pending tasks and the empty data rows of a new sale, inside the caller's
// transaction.
func CreateInitial(ctx context.Context, q querier, saleID, actor uuid.UUID, now time.Time) error {
	for n := 1; n <= TaskCount; n++ {
		if _, err := q.Exec(ctx, `INSERT INTO task_instances (id, sale_id, task_number, created_by, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $4)`, saleID, n, actor, now); err != nil {
			return err
		}
	}
	for _, table := range []string{"sale_financials", "sale_due_diligence", "sale_fulfillment"} {
		if _, err := q.Exec(ctx, `INSERT INTO `+table+` (sale_id) VALUES ($1)`, saleID); err != nil {
			return err
		}
	}
	return nil
}

// ShiftDueDate moves the due date of the task in progress forward. A cancelled sale that is reopened resumes
// with the time it had left, so its due date moves by the time spent cancelled (S5). The start time stays as it
// was: the completion phase and the reminders work from the due date.
func ShiftDueDate(ctx context.Context, q Querier, saleID, actor uuid.UUID, by time.Duration, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE task_instances SET due_date = due_date + make_interval(secs => $2::double precision),
			updated_by = $3, updated_at = $4, row_version = row_version + 1
		WHERE sale_id = $1 AND status = 'in_progress' AND due_date IS NOT NULL`, saleID, by.Seconds(), actor, now)
	return err
}

// TaskInfo is the little the documents module needs to know about a task.
type TaskInfo struct {
	ID     uuid.UUID
	Number int
	Status string
}

// FindTask returns a task of a sale, or (nil, nil) when it does not exist. With forUpdate the task row is locked.
func FindTask(ctx context.Context, q Querier, saleID uuid.UUID, number int, forUpdate bool) (*TaskInfo, error) {
	sql := `SELECT id, task_number, status::text FROM task_instances WHERE sale_id = $1 AND task_number = $2`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	var t TaskInfo
	err := q.QueryRow(ctx, sql, saleID, number).Scan(&t.ID, &t.Number, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}
