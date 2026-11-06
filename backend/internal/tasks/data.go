package tasks

import (
	"context"
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// kind is the data side of one task: what it shows, how a draft is saved and what must be true to complete it.
type kind interface {
	// load returns the task's data as the API shows it.
	load(ctx context.Context, q Querier, sale *SaleRef) (any, error)
	// save applies a draft (a partial update). Problems that block the save come back in the draft.
	save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, gate SaleGate) (*draft, error)
	// check returns every field problem that blocks completion. It may refresh derived values.
	check(ctx context.Context, q Querier, sale *SaleRef, actor uuid.UUID, now time.Time) ([]apperr.FieldError, error)
}

// kindFor returns the data handler of a task number (1 to 13).
func kindFor(number int) kind {
	switch number {
	case 1:
		return task1
	case 2:
		return task2{}
	case 3:
		return task3{}
	case 4:
		return payment{kind: "pana", expectedLabel: "el precio final cotizado"}
	case 5:
		return payment{kind: "toyota", expectedLabel: "el monto final a pagar a Toyota"}
	case 6:
		return invoices{}
	case 7:
		return task7{}
	case 8:
		return task8
	case 9:
		return task9
	case 10:
		return task10
	case 11:
		return task11
	case 12:
		return task12
	case 13:
		return task13
	}
	return nil
}

// requiredSlots lists the document slots a task needs before it can be completed. Power of attorney
// (Task 2) is only needed when the customer is not a Diplomático.
func requiredSlots(number int, customerType string) []string {
	switch number {
	case 1:
		return []string{"quotation_pdf", "quotation_excel"}
	case 2:
		slots := []string{"signer_identity_scan", "customer_knowledge_affidavit"}
		if customerType != "diplomatico" {
			slots = append(slots, "power_of_attorney")
		}
		return slots
	case 3:
		return []string{"unit_settlement"}
	case 4:
		return []string{"pana_payment_voucher"}
	case 5:
		return []string{"toyota_payment_voucher", "toyota_payment_confirmation"}
	case 6:
		return []string{"invoice_toyota", "invoice_commission", "invoice_accessories", "invoice_release_processing"}
	case 7:
		return []string{"endorsed_documents"}
	case 8:
		return []string{"deposit_order", "customs_certificate"}
	case 10:
		return []string{"tive_document"}
	case 12:
		return []string{"service_order"}
	case 13:
		return []string{"signed_delivery_record"}
	}
	return []string{}
}

// update builds an UPDATE of one row keyed by sale_id. $1 is the actor, $2 the time and $3 the sale.
type update struct {
	table string
	sets  []string
	args  []any
}

func newUpdate(table string, saleID, actor uuid.UUID, now time.Time) *update {
	return &update{table: table, args: []any{actor, now, saleID}}
}

// set adds one column. cast is the SQL cast of the parameter, such as "::text::numeric".
func (u *update) set(column, cast string, value any) {
	u.args = append(u.args, value)
	u.sets = append(u.sets, fmt.Sprintf("%s = $%d%s", column, len(u.args), cast))
}

func (u *update) exec(ctx context.Context, q Querier) error {
	if len(u.sets) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, "UPDATE "+u.table+" SET "+strings.Join(u.sets, ", ")+
		", updated_by = $1, updated_at = $2, row_version = row_version + 1 WHERE sale_id = $3", u.args...)
	return err
}

// How a column is read and validated.
type colKind int

const (
	kText     colKind = iota // free text, trimmed
	kMoney                   // an amount in USD with two decimals
	kDate                    // a calendar date
	kCode                    // one value of a fixed list
	kIdent                   // an identifier stored on the sale (series, VIN or plate)
	kReadOnly                // calculated by the database; shown but never written
)

// column describes one field of a task whose data lives in a one-row-per-sale table.
type column struct {
	key      string // name in the API
	label    string // Spanish name used in messages, with its article ("El monto inicial")
	col      string // database column
	kind     colKind
	cast     string // cast of the SQL parameter
	max      int    // longest text, in characters
	allowed  []string
	ident    Identifier
	required bool
	compact  bool // upper-case and strip spaces before checking (numbers the database stores normalized)
}

// tableKind is a task whose data is a set of columns of one row of sale_financials, sale_due_diligence or
// sale_fulfillment. Tasks with several rows (payments, invoices) or special rules have their own type.
type tableKind struct {
	table string
	cols  []column
	// after runs once the new values are known, to report out-of-order dates and similar warnings.
	after func(d *draft, next map[string]*string)
}

func (k tableKind) selectable() []column {
	var out []column
	for _, c := range k.cols {
		if c.kind != kIdent {
			out = append(out, c)
		}
	}
	return out
}

// read returns the current value of every column, as text. Identifiers come from the sale.
func (k tableKind) read(ctx context.Context, q Querier, sale *SaleRef) (map[string]*string, error) {
	values := map[string]*string{}
	cols := k.selectable()
	if len(cols) > 0 {
		names := make([]string, len(cols))
		dest := make([]any, len(cols))
		cells := make([]*string, len(cols))
		for i, c := range cols {
			names[i] = c.col + "::text"
			dest[i] = &cells[i]
		}
		sql := "SELECT " + strings.Join(names, ", ") + " FROM " + k.table + " WHERE sale_id = $1"
		if err := q.QueryRow(ctx, sql, sale.ID).Scan(dest...); err != nil {
			return nil, err
		}
		for i, c := range cols {
			values[c.key] = cells[i]
		}
	}
	for _, c := range k.cols {
		if c.kind != kIdent {
			continue
		}
		switch c.ident {
		case IdentSeries:
			values[c.key] = sale.Series
		case IdentVIN:
			values[c.key] = sale.VIN
		case IdentPlate:
			values[c.key] = sale.Plate
		}
	}
	return values, nil
}

func (k tableKind) load(ctx context.Context, q Querier, sale *SaleRef) (any, error) {
	values, err := k.read(ctx, q, sale)
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (k tableKind) save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, gate SaleGate) (*draft, error) {
	fields := map[string]json.RawMessage{}
	if err := decodeStrict(raw, &fields); err != nil {
		return nil, err
	}
	known := map[string]column{}
	for _, c := range k.cols {
		known[c.key] = c
	}
	for key := range fields {
		c, ok := known[key]
		switch {
		case !ok:
			return nil, apperr.BadRequest("INVALID_JSON", "El campo "+key+" no existe en esta tarea.")
		case c.kind == kReadOnly:
			return nil, apperr.BadRequest("INVALID_JSON", "El campo "+key+" se calcula y no se puede modificar.")
		}
	}

	current, err := k.read(ctx, q, sale)
	if err != nil {
		return nil, err
	}
	next := map[string]*string{}
	for key, value := range current {
		next[key] = value
	}
	d := newDraft(sale)
	u := newUpdate(k.table, sale.ID, actor, now)
	type identWrite struct {
		ident Identifier
		value *string
	}
	var identWrites []identWrite

	for _, c := range k.cols {
		rawValue, sent := fields[c.key]
		if !sent {
			continue
		}
		var value *string
		var set bool
		switch c.kind {
		case kMoney:
			var f Field[Amount]
			if err := f.UnmarshalJSON(rawValue); err != nil {
				return nil, apperr.BadRequest("INVALID_JSON", "El valor de "+c.key+" no es válido.")
			}
			value, set = d.money(f, c.key, c.label)
		default:
			var f Field[string]
			if err := f.UnmarshalJSON(rawValue); err != nil {
				return nil, apperr.BadRequest("INVALID_JSON", "El valor de "+c.key+" no es válido.")
			}
			switch c.kind {
			case kDate:
				value, set = d.date(f, c.key, c.label)
			case kCode:
				value, set = d.code(f, c.key, c.label, c.allowed...)
			case kIdent:
				value, set = d.identifier(f, c.key, c.label, c.ident)
			default:
				value, set = d.text(f, c.key, c.label, c.max)
				if set && value != nil && c.compact {
					compacted := compact(*value)
					value = &compacted
				}
			}
		}
		if !set {
			continue
		}
		next[c.key] = value
		d.note(c.key, current[c.key], value, false)
		if c.kind == kIdent {
			identWrites = append(identWrites, identWrite{c.ident, value})
		} else {
			u.set(c.col, c.cast, value)
		}
	}
	if k.after != nil {
		k.after(d, next)
	}
	if d.result() != nil {
		return d, nil
	}
	if err := u.exec(ctx, q); err != nil {
		return nil, err
	}
	for _, w := range identWrites {
		if err := gate.SetIdentifier(ctx, q, sale.ID, w.ident, w.value, actor, now); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (k tableKind) check(ctx context.Context, q Querier, sale *SaleRef, _ uuid.UUID, _ time.Time) ([]apperr.FieldError, error) {
	values, err := k.read(ctx, q, sale)
	if err != nil {
		return nil, err
	}
	var problems []apperr.FieldError
	for _, c := range k.cols {
		if c.required {
			problems = required(problems, c.key, c.label, values[c.key] != nil)
		}
	}
	return problems, nil
}

// identifier validates and normalizes a vehicle identifier (series, VIN or plate).
func (d *draft) identifier(f Field[string], field, label string, ident Identifier) (*string, bool) {
	if !f.Set {
		return nil, false
	}
	if f.Value == nil || strings.TrimSpace(*f.Value) == "" {
		return nil, true
	}
	value := compact(*f.Value)
	switch ident {
	case IdentSeries:
		if len(value) > 40 {
			d.fail(field, "TOO_LONG", label+" es demasiado largo (máximo 40 caracteres).")
			return nil, false
		}
	case IdentVIN:
		if !vinPattern.MatchString(value) {
			d.fail(field, "VIN_INVALID", "El VIN debe tener 17 caracteres y no puede incluir las letras I, O ni Q.")
			return nil, false
		}
	case IdentPlate:
		if !platePattern.MatchString(value) {
			d.fail(field, "PLATE_INVALID", "La placa debe tener 6 caracteres alfanuméricos.")
			return nil, false
		}
	}
	return &value, true
}

// The tasks that only need a set of columns.
var (
	task1 = tableKind{table: "sale_financials", cols: []column{
		{key: "initialToyotaAmount", label: "El monto inicial a pagar a Toyota", col: "initial_toyota_amount", kind: kMoney, cast: "::text::numeric", required: true},
		{key: "initialCommission", label: "La comisión inicial de Grupo Pana", col: "initial_commission", kind: kMoney, cast: "::text::numeric", required: true},
		{key: "accessoryAmount", label: "El monto de accesorios", col: "accessory_amount", kind: kMoney, cast: "::text::numeric", required: true},
		{key: "releaseProcessingAmount", label: "El monto del trámite de liberación del vehículo", col: "release_processing_amount", kind: kMoney, cast: "::text::numeric", required: true},
		{key: "quotedFinalPrice", label: "El precio final cotizado", col: "quoted_final_price", kind: kReadOnly},
	}}

	task8 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "damNumber", label: "El número de DAM", col: "dam_number", kind: kText, max: 40, compact: true, required: true},
		{
			key: "customsOffice", label: "La intendencia de aduana", col: "customs_office", kind: kCode, cast: "::text::customs_office",
			allowed: []string{"callao_maritima", "chancay"}, required: true,
		},
		{
			key: "controlChannel", label: "El canal de control", col: "control_channel", kind: kCode, cast: "::text::control_channel",
			allowed: []string{"green", "orange", "red"}, required: true,
		},
	}}

	task9 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "personInCharge", label: "El responsable", col: "release_person_in_charge", kind: kText, max: 120, required: true},
	}}

	task10 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "licensePlate", label: "La placa", col: "license_plate", kind: kIdent, ident: IdentPlate, required: true},
		{key: "registrationDate", label: "La fecha de inmatriculación", col: "registration_date", kind: kDate, cast: "::text::date", required: true},
	}}

	task11 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "transferDate", label: "La fecha de traslado a PDS", col: "pds_transfer_date", kind: kDate, cast: "::text::date", required: true},
	}}

	task12 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "serviceOrderNumber", label: "El número de orden de servicio", col: "equipment_service_order_number", kind: kText, max: 40, required: true},
		{key: "completionDate", label: "La fecha de término del equipamiento", col: "equipment_completion_date", kind: kDate, cast: "::text::date", required: true},
	}}

	task13 = tableKind{table: "sale_fulfillment", cols: []column{
		{key: "surquilloTransferDate", label: "La fecha de traslado a Surquillo", col: "surquillo_transfer_date", kind: kDate, cast: "::text::date"},
		{key: "deliveryDate", label: "La fecha de entrega del vehículo", col: "vehicle_delivery_date", kind: kDate, cast: "::text::date", required: true},
	}, after: func(d *draft, next map[string]*string) {
		transfer, delivery := next["surquilloTransferDate"], next["deliveryDate"]
		if transfer != nil && delivery != nil && *delivery < *transfer { // YYYY-MM-DD sorts as a date
			d.warn("deliveryDate", "DATE_OUT_OF_ORDER", "La fecha de entrega es anterior a la fecha de traslado a Surquillo.")
		}
	}}
)

// task7 collects no fields: its only requirement is the endorsed documents file.
type task7 struct{}

func (task7) load(context.Context, Querier, *SaleRef) (any, error) { return struct{}{}, nil }

func (task7) save(_ context.Context, _ Querier, sale *SaleRef, raw json.RawMessage, _ uuid.UUID, _ time.Time, _ SaleGate) (*draft, error) {
	var none struct{}
	if err := decodeStrict(raw, &none); err != nil {
		return nil, err
	}
	return newDraft(sale), nil
}

func (task7) check(context.Context, Querier, *SaleRef, uuid.UUID, time.Time) ([]apperr.FieldError, error) {
	return nil, nil
}
