package tasks

import (
	"context"
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/ids"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Task 6: the four official invoices. Each one has a number (series, hyphen, correlative), a date and an
// amount, and is expected to equal an amount fixed by Task 1 or Task 3.

type invoices struct{}

// invoiceKinds lists the kinds in the order the screen shows them. kind is the invoice_kind value.
var invoiceKinds = []struct{ kind, key, label string }{
	{"toyota", "toyota", "la factura de Toyota"},
	{"commission", "commission", "la factura de comisión"},
	{"accessories", "accessories", "la factura de accesorios"},
	{"release_processing", "releaseProcessing", "la factura del trámite de liberación"},
}

type invoicePatch struct {
	Number         Field[string] `json:"number"`
	Date           Field[string] `json:"date"`
	Amount         Field[Amount] `json:"amount"`
	MismatchReason Field[string] `json:"mismatchReason"`
}

type invoicesPatch struct {
	Toyota            *invoicePatch `json:"toyota"`
	Commission        *invoicePatch `json:"commission"`
	Accessories       *invoicePatch `json:"accessories"`
	ReleaseProcessing *invoicePatch `json:"releaseProcessing"`
}

func (p invoicesPatch) forKey(key string) *invoicePatch {
	switch key {
	case "toyota":
		return p.Toyota
	case "commission":
		return p.Commission
	case "accessories":
		return p.Accessories
	case "releaseProcessing":
		return p.ReleaseProcessing
	}
	return nil
}

type invoiceRow struct {
	Number, Date, Amount, Expected, MismatchReason *string
}

type invoiceData struct {
	Number         *string `json:"number"`
	Date           *string `json:"date"`
	Amount         *string `json:"amount"`
	ExpectedAmount *string `json:"expectedAmount"`
	MismatchReason *string `json:"mismatchReason"`
}

type invoicesData struct {
	Toyota            invoiceData `json:"toyota"`
	Commission        invoiceData `json:"commission"`
	Accessories       invoiceData `json:"accessories"`
	ReleaseProcessing invoiceData `json:"releaseProcessing"`
}

// invoiceExpected is the amount an invoice has to match.
func invoiceExpected(f *financials, kind string) *string {
	switch kind {
	case "toyota":
		return f.FinalToyota
	case "commission":
		return f.FinalCommission
	case "accessories":
		return f.Accessory
	case "release_processing":
		return f.Release
	}
	return nil
}

func readInvoice(ctx context.Context, q Querier, saleID uuid.UUID, kind string) (invoiceRow, error) {
	var r invoiceRow
	err := q.QueryRow(ctx, `SELECT invoice_number, invoice_date::text, invoice_amount::text, expected_amount::text, mismatch_reason
		FROM sale_invoices WHERE sale_id = $1 AND invoice_kind = $2::text::invoice_kind`, saleID, kind).
		Scan(&r.Number, &r.Date, &r.Amount, &r.Expected, &r.MismatchReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoiceRow{}, nil
	}
	return r, err
}

func (invoices) load(ctx context.Context, q Querier, sale *SaleRef) (any, error) {
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	var out invoicesData
	targets := map[string]*invoiceData{
		"toyota": &out.Toyota, "commission": &out.Commission,
		"accessories": &out.Accessories, "releaseProcessing": &out.ReleaseProcessing,
	}
	for _, k := range invoiceKinds {
		r, err := readInvoice(ctx, q, sale.ID, k.kind)
		if err != nil {
			return nil, err
		}
		*targets[k.key] = invoiceData{r.Number, r.Date, r.Amount, invoiceExpected(f, k.kind), r.MismatchReason}
	}
	return out, nil
}

func (invoices) save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, _ SaleGate) (*draft, error) {
	var in invoicesPatch
	if err := decodeStrict(raw, &in); err != nil {
		return nil, err
	}
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	d := newDraft(sale)
	type write struct {
		kind string
		row  invoiceRow
	}
	var writes []write

	for _, k := range invoiceKinds {
		patch := in.forKey(k.key)
		if patch == nil {
			continue
		}
		cur, err := readInvoice(ctx, q, sale.ID, k.kind)
		if err != nil {
			return nil, err
		}
		next := cur
		changed := false
		prefix := k.key + "."

		if patch.Number.Set {
			var number *string
			if patch.Number.Value != nil {
				if c := compact(*patch.Number.Value); c != "" {
					number = &c
				}
			}
			if number != nil && !invoicePattern.MatchString(*number) {
				d.fail(prefix+"number", "INVOICE_NUMBER_INVALID",
					"El número de factura debe tener serie de 4 caracteres, un guion y el correlativo (por ejemplo F001-000123).")
			} else {
				next.Number, changed = number, true
				d.note(prefix+"number", cur.Number, number, false)
			}
		}
		if v, ok := d.date(patch.Date, prefix+"date", "La fecha de la factura"); ok {
			next.Date, changed = v, true
			d.note(prefix+"date", cur.Date, v, false)
		}
		if v, ok := d.money(patch.Amount, prefix+"amount", "El monto de la factura"); ok {
			next.Amount, changed = v, true
			d.note(prefix+"amount", cur.Amount, v, false)
		}
		if v, ok := d.text(patch.MismatchReason, prefix+"mismatchReason", "El motivo de la diferencia", 500); ok {
			next.MismatchReason, changed = v, true
			d.note(prefix+"mismatchReason", cur.MismatchReason, v, false)
		}
		if !changed {
			continue
		}
		expected := invoiceExpected(f, k.kind)
		next.Expected = expected
		next.MismatchReason = reconcileMismatch(d, prefix+"mismatchReason", next.Amount, expected, next.MismatchReason, "el monto esperado")
		writes = append(writes, write{k.kind, next})
	}
	if d.result() != nil {
		return d, nil
	}
	for _, w := range writes {
		_, err := q.Exec(ctx, `INSERT INTO sale_invoices
			(id, sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount, mismatch_reason,
			 created_at, updated_by, updated_at)
			VALUES ($1, $2, $3::text::invoice_kind, $4, $5::text::date, $6::text::numeric, $7::text::numeric, $8, $9, $10, $9)
			ON CONFLICT (sale_id, invoice_kind) DO UPDATE SET
			  invoice_number = EXCLUDED.invoice_number, invoice_date = EXCLUDED.invoice_date,
			  invoice_amount = EXCLUDED.invoice_amount, expected_amount = EXCLUDED.expected_amount,
			  mismatch_reason = EXCLUDED.mismatch_reason, updated_by = EXCLUDED.updated_by,
			  updated_at = EXCLUDED.updated_at, row_version = sale_invoices.row_version + 1`,
			ids.New(), sale.ID, w.kind, w.row.Number, w.row.Date, w.row.Amount, w.row.Expected,
			w.row.MismatchReason, now, actor)
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (invoices) check(ctx context.Context, q Querier, sale *SaleRef, actor uuid.UUID, now time.Time) ([]apperr.FieldError, error) {
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	var problems []apperr.FieldError
	type refresh struct {
		kind     string
		expected *string
	}
	var refreshes []refresh
	for _, k := range invoiceKinds {
		r, err := readInvoice(ctx, q, sale.ID, k.kind)
		if err != nil {
			return nil, err
		}
		expected := invoiceExpected(f, k.kind)
		before := len(problems)
		prefix := k.key + "."
		problems = required(problems, prefix+"number", "El número de la factura", r.Number != nil)
		problems = required(problems, prefix+"date", "La fecha de la factura", r.Date != nil)
		problems = required(problems, prefix+"amount", "El monto de la factura", r.Amount != nil)
		if differs(r.Amount, expected) && r.MismatchReason == nil {
			problems = append(problems, apperr.FieldError{
				Field: prefix + "mismatchReason", Code: "MISMATCH_REASON_REQUIRED",
				Message: "El monto de " + k.label + " no coincide con el esperado: escriba el motivo de la diferencia.",
			})
		}
		if len(problems) == before && !equalText(r.Expected, expected) {
			refreshes = append(refreshes, refresh{k.kind, expected})
		}
	}
	// The expected amounts stored with the rows are exactly what this check compared.
	if len(problems) == 0 {
		for _, r := range refreshes {
			_, err := q.Exec(ctx, `UPDATE sale_invoices SET expected_amount = $3::text::numeric, updated_by = $4, updated_at = $5,
				row_version = row_version + 1 WHERE sale_id = $1 AND invoice_kind = $2::text::invoice_kind`,
				sale.ID, r.kind, r.expected, actor, now)
			if err != nil {
				return nil, err
			}
		}
	}
	return problems, nil
}
