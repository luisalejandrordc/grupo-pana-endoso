package tasks

import (
	"context"
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/ids"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// financials are the amounts of Tasks 1 and 3, as the database returns them (decimal text).
type financials struct {
	InitialToyota, InitialCommission, Accessory, Release *string
	Quoted, FinalToyota, FinalCommission                 *string
}

func loadFinancials(ctx context.Context, q Querier, saleID uuid.UUID) (*financials, error) {
	var f financials
	err := q.QueryRow(ctx, `SELECT initial_toyota_amount::text, initial_commission::text, accessory_amount::text,
		release_processing_amount::text, quoted_final_price::text, final_toyota_amount::text, final_commission::text
		FROM sale_financials WHERE sale_id = $1`, saleID).
		Scan(&f.InitialToyota, &f.InitialCommission, &f.Accessory, &f.Release, &f.Quoted, &f.FinalToyota, &f.FinalCommission)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// differs reports whether two amounts are both known and not exactly equal.
func differs(a, b *string) bool {
	x, okA := centsOf(a)
	y, okB := centsOf(b)
	return okA && okB && x != y
}

// ---------------------------------------------------------------------------------------------------
// Task 2: due diligence. The signer's document is a pair (type and number) that is saved together.

type task2 struct{}

type task2Patch struct {
	SignerDocumentType   Field[string] `json:"signerDocumentType"`
	SignerDocumentNumber Field[string] `json:"signerDocumentNumber"`
	SignerFirstName      Field[string] `json:"signerFirstName"`
	SignerLastName       Field[string] `json:"signerLastName"`
	SignerNameSource     Field[string] `json:"signerNameSource"`
	PepStatus            Field[string] `json:"pepStatus"`
	DdcRegime            Field[string] `json:"ddcRegime"`
}

type task2Data struct {
	SignerDocumentType   *string `json:"signerDocumentType"`
	SignerDocumentNumber *string `json:"signerDocumentNumber"`
	SignerFirstName      *string `json:"signerFirstName"`
	SignerLastName       *string `json:"signerLastName"`
	SignerNameSource     *string `json:"signerNameSource"`
	PepStatus            *string `json:"pepStatus"`
	DdcRegime            *string `json:"ddcRegime"`
}

func readTask2(ctx context.Context, q Querier, saleID uuid.UUID) (task2Data, error) {
	var r task2Data
	err := q.QueryRow(ctx, `SELECT signer_document_type::text, signer_document_number, signer_first_name, signer_last_name,
		signer_name_source::text, pep_status::text, ddc_regime::text FROM sale_due_diligence WHERE sale_id = $1`, saleID).
		Scan(&r.SignerDocumentType, &r.SignerDocumentNumber, &r.SignerFirstName, &r.SignerLastName,
			&r.SignerNameSource, &r.PepStatus, &r.DdcRegime)
	return r, err
}

func (task2) load(ctx context.Context, q Querier, sale *SaleRef) (any, error) {
	return readTask2(ctx, q, sale.ID)
}

func (task2) save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, _ SaleGate) (*draft, error) {
	var in task2Patch
	if err := decodeStrict(raw, &in); err != nil {
		return nil, err
	}
	cur, err := readTask2(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	d := newDraft(sale)
	u := newUpdate("sale_due_diligence", sale.ID, actor, now)

	// The document type and number change together: one without the other is refused.
	if in.SignerDocumentType.Set || in.SignerDocumentNumber.Set {
		docType, docNumber := cur.SignerDocumentType, cur.SignerDocumentNumber
		if in.SignerDocumentType.Set {
			docType = cleanText(in.SignerDocumentType.Value)
		}
		if in.SignerDocumentNumber.Set {
			docNumber = cleanText(in.SignerDocumentNumber.Value)
		}
		switch {
		case docType == nil && docNumber == nil:
			u.set("signer_document_type", "::text::customer_document_type", nil)
			u.set("signer_document_number", "", nil)
			d.note("signerDocumentNumber", cur.SignerDocumentNumber, nil, true)
		case docType == nil || docNumber == nil:
			d.fail("signerDocumentNumber", "DOCUMENT_PAIR_REQUIRED", "Indique el tipo y el número de documento del firmante.")
		default:
			t, n, ok := normalizeSignerDocument(*docType, *docNumber)
			if !ok {
				d.fail("signerDocumentNumber", "INVALID_DOCUMENT", "El número de documento no es válido para su tipo.")
				break
			}
			u.set("signer_document_type", "::text::customer_document_type", t)
			u.set("signer_document_number", "", n)
			d.note("signerDocumentType", cur.SignerDocumentType, &t, false)
			d.note("signerDocumentNumber", cur.SignerDocumentNumber, &n, true)
		}
	}

	first, firstSet := d.text(in.SignerFirstName, "signerFirstName", "El nombre del firmante", 100)
	if firstSet {
		u.set("signer_first_name", "", first)
		d.note("signerFirstName", cur.SignerFirstName, first, true)
	}
	last, lastSet := d.text(in.SignerLastName, "signerLastName", "El apellido del firmante", 100)
	if lastSet {
		u.set("signer_last_name", "", last)
		d.note("signerLastName", cur.SignerLastName, last, true)
	}
	source, sourceSet := d.code(in.SignerNameSource, "signerNameSource", "El origen del nombre", "looked_up", "manual")
	switch {
	case sourceSet:
		u.set("signer_name_source", "::text::name_source", source)
		d.note("signerNameSource", cur.SignerNameSource, source, false)
	case firstSet || lastSet:
		// A name typed by hand is no longer "looked up", unless the caller says otherwise.
		manual := "manual"
		u.set("signer_name_source", "::text::name_source", manual)
		d.note("signerNameSource", cur.SignerNameSource, &manual, false)
	}
	if v, ok := d.code(in.PepStatus, "pepStatus", "El estado PEP", "no_pep", "pep", "related_to_pep"); ok {
		u.set("pep_status", "::text::pep_status", v)
		d.note("pepStatus", cur.PepStatus, v, false)
	}
	if v, ok := d.code(in.DdcRegime, "ddcRegime", "El régimen de debida diligencia", "general", "reinforced"); ok {
		u.set("ddc_regime", "::text::ddc_regime", v)
		d.note("ddcRegime", cur.DdcRegime, v, false)
	}
	if d.result() != nil {
		return d, nil
	}
	return d, u.exec(ctx, q)
}

func (task2) check(ctx context.Context, q Querier, sale *SaleRef, _ uuid.UUID, _ time.Time) ([]apperr.FieldError, error) {
	r, err := readTask2(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	var p []apperr.FieldError
	p = required(p, "signerDocumentType", "El tipo de documento del firmante", r.SignerDocumentType != nil)
	p = required(p, "signerDocumentNumber", "El número de documento del firmante", r.SignerDocumentNumber != nil)
	p = required(p, "signerFirstName", "El nombre del firmante", r.SignerFirstName != nil)
	p = required(p, "signerLastName", "El apellido del firmante", r.SignerLastName != nil)
	p = required(p, "pepStatus", "El estado PEP", r.PepStatus != nil)
	p = required(p, "ddcRegime", "El régimen de debida diligencia", r.DdcRegime != nil)
	return p, nil
}

// cleanText trims a value and turns empty text into nil.
func cleanText(p *string) *string {
	if p == nil {
		return nil
	}
	value := strings.TrimSpace(*p)
	if value == "" {
		return nil
	}
	return &value
}

// ---------------------------------------------------------------------------------------------------
// Task 3: unit settlement. It sets the series and VIN of the vehicle and the final amount payable to Toyota;
// the final commission is calculated by the database.

type task3 struct{}

type task3Patch struct {
	VehicleSeries     Field[string] `json:"vehicleSeries"`
	Vin               Field[string] `json:"vin"`
	FinalToyotaAmount Field[Amount] `json:"finalToyotaAmount"`
}

type task3Data struct {
	VehicleSeries     *string `json:"vehicleSeries"`
	Vin               *string `json:"vin"`
	FinalToyotaAmount *string `json:"finalToyotaAmount"`
	FinalCommission   *string `json:"finalCommission"`
}

func (task3) load(ctx context.Context, q Querier, sale *SaleRef) (any, error) {
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	return task3Data{VehicleSeries: sale.Series, Vin: sale.VIN, FinalToyotaAmount: f.FinalToyota, FinalCommission: f.FinalCommission}, nil
}

func (task3) save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, gate SaleGate) (*draft, error) {
	var in task3Patch
	if err := decodeStrict(raw, &in); err != nil {
		return nil, err
	}
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	d := newDraft(sale)
	u := newUpdate("sale_financials", sale.ID, actor, now)

	series, seriesSet := d.identifier(in.VehicleSeries, "vehicleSeries", "El número de serie", IdentSeries)
	if seriesSet {
		d.note("vehicleSeries", sale.Series, series, false)
	}
	vin, vinSet := d.identifier(in.Vin, "vin", "El VIN", IdentVIN)
	if vinSet {
		d.note("vin", sale.VIN, vin, false)
	}
	if v, ok := d.money(in.FinalToyotaAmount, "finalToyotaAmount", "El monto final a pagar a Toyota"); ok {
		if v != nil {
			// final commission = quoted price - final Toyota amount - accessories - release processing
			quoted, ok1 := centsOf(f.Quoted)
			accessory, ok2 := centsOf(f.Accessory)
			release, ok3 := centsOf(f.Release)
			final, _ := parseMoney(*v)
			if ok1 && ok2 && ok3 && quoted-final-accessory-release <= 0 {
				d.fail("finalToyotaAmount", "COMMISSION_NOT_POSITIVE",
					"La comisión final debe ser mayor que cero. Revise el monto final a pagar a Toyota.")
			}
		}
		u.set("final_toyota_amount", "::text::numeric", v)
		d.note("finalToyotaAmount", f.FinalToyota, v, false)
	}
	if d.result() != nil {
		return d, nil
	}
	if err := u.exec(ctx, q); err != nil {
		return nil, err
	}
	if seriesSet {
		if err := gate.SetIdentifier(ctx, q, sale.ID, IdentSeries, series, actor, now); err != nil {
			return nil, err
		}
	}
	if vinSet {
		if err := gate.SetIdentifier(ctx, q, sale.ID, IdentVIN, vin, actor, now); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (task3) check(ctx context.Context, q Querier, sale *SaleRef, _ uuid.UUID, _ time.Time) ([]apperr.FieldError, error) {
	f, err := loadFinancials(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	var p []apperr.FieldError
	p = required(p, "vehicleSeries", "El número de serie", sale.Series != nil)
	p = required(p, "vin", "El VIN", sale.VIN != nil)
	p = required(p, "finalToyotaAmount", "El monto final a pagar a Toyota", f.FinalToyota != nil)
	if f.FinalToyota != nil && f.FinalCommission == nil {
		p = append(p, apperr.FieldError{
			Field: "finalToyotaAmount", Code: "COMMISSION_NOT_POSITIVE",
			Message: "La comisión final debe ser mayor que cero.",
		})
	}
	return p, nil
}

// ---------------------------------------------------------------------------------------------------
// Tasks 4 and 5: a payment. Task 4 is the customer's payment to Grupo Pana, expected to equal the quoted
// final price; Task 5 is the payment to Toyota, expected to equal the final Toyota amount.

type payment struct {
	kind          string // payment_kind: pana or toyota
	expectedLabel string // what the amount is compared with, for messages
}

type paymentRow struct {
	Bank, PaymentDate, TransactionNumber, Deposited, Expected, MismatchReason *string
}

type paymentPatch struct {
	Bank              Field[string] `json:"bank"`
	PaymentDate       Field[string] `json:"paymentDate"`
	TransactionNumber Field[string] `json:"transactionNumber"`
	DepositedAmount   Field[Amount] `json:"depositedAmount"`
	MismatchReason    Field[string] `json:"mismatchReason"`
}

type paymentData struct {
	Bank              *string `json:"bank"`
	PaymentDate       *string `json:"paymentDate"`
	TransactionNumber *string `json:"transactionNumber"`
	DepositedAmount   *string `json:"depositedAmount"`
	ExpectedAmount    *string `json:"expectedAmount"`
	MismatchReason    *string `json:"mismatchReason"`
}

var allBanks = []string{"bcp", "bbva", "scotiabank", "interbank"}

func (p payment) expected(ctx context.Context, q Querier, saleID uuid.UUID) (*string, error) {
	f, err := loadFinancials(ctx, q, saleID)
	if err != nil {
		return nil, err
	}
	if p.kind == "pana" {
		return f.Quoted, nil
	}
	return f.FinalToyota, nil
}

// read returns the stored row, or an empty one when the task has not saved anything yet.
func (p payment) read(ctx context.Context, q Querier, saleID uuid.UUID) (paymentRow, error) {
	var r paymentRow
	err := q.QueryRow(ctx, `SELECT bank::text, payment_date::text, transaction_number, deposited_amount::text,
		expected_amount::text, mismatch_reason FROM sale_payments
		WHERE sale_id = $1 AND payment_kind = $2::text::payment_kind`, saleID, p.kind).
		Scan(&r.Bank, &r.PaymentDate, &r.TransactionNumber, &r.Deposited, &r.Expected, &r.MismatchReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentRow{}, nil
	}
	return r, err
}

func (p payment) load(ctx context.Context, q Querier, sale *SaleRef) (any, error) {
	r, err := p.read(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	expected, err := p.expected(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	return paymentData{r.Bank, r.PaymentDate, r.TransactionNumber, r.Deposited, expected, r.MismatchReason}, nil
}

func (p payment) save(ctx context.Context, q Querier, sale *SaleRef, raw json.RawMessage, actor uuid.UUID, now time.Time, _ SaleGate) (*draft, error) {
	var in paymentPatch
	if err := decodeStrict(raw, &in); err != nil {
		return nil, err
	}
	cur, err := p.read(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	expected, err := p.expected(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	d := newDraft(sale)
	next := cur
	changed := false

	if v, ok := d.code(in.Bank, "bank", "El banco", allBanks...); ok {
		next.Bank, changed = v, true
		d.note("bank", cur.Bank, v, false)
	}
	if v, ok := d.date(in.PaymentDate, "paymentDate", "La fecha de la operación"); ok {
		next.PaymentDate, changed = v, true
		d.note("paymentDate", cur.PaymentDate, v, false)
	}
	if v, ok := d.text(in.TransactionNumber, "transactionNumber", "El número de operación", 40); ok {
		if v != nil {
			c := compact(*v)
			v = &c
		}
		next.TransactionNumber, changed = v, true
		d.note("transactionNumber", cur.TransactionNumber, v, false)
	}
	if v, ok := d.money(in.DepositedAmount, "depositedAmount", "El monto depositado"); ok {
		next.Deposited, changed = v, true
		d.note("depositedAmount", cur.Deposited, v, false)
	}
	if v, ok := d.text(in.MismatchReason, "mismatchReason", "El motivo de la diferencia", 500); ok {
		next.MismatchReason, changed = v, true
		d.note("mismatchReason", cur.MismatchReason, v, false)
	}
	if !changed {
		return d, nil
	}
	next.Expected = expected
	next.MismatchReason = reconcileMismatch(d, "mismatchReason", next.Deposited, expected, next.MismatchReason, p.expectedLabel)
	if d.result() != nil {
		return d, nil
	}
	_, err = q.Exec(ctx, `INSERT INTO sale_payments
		(id, sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount,
		 mismatch_reason, created_at, updated_by, updated_at)
		VALUES ($1, $2, $3::text::payment_kind, $4::text::bank, $5::text::date, $6, $7::text::numeric,
		        $8::text::numeric, $9, $10, $11, $10)
		ON CONFLICT (sale_id, payment_kind) DO UPDATE SET
		  bank = EXCLUDED.bank, payment_date = EXCLUDED.payment_date, transaction_number = EXCLUDED.transaction_number,
		  deposited_amount = EXCLUDED.deposited_amount, expected_amount = EXCLUDED.expected_amount,
		  mismatch_reason = EXCLUDED.mismatch_reason, updated_by = EXCLUDED.updated_by,
		  updated_at = EXCLUDED.updated_at, row_version = sale_payments.row_version + 1`,
		ids.New(), sale.ID, p.kind, next.Bank, next.PaymentDate, next.TransactionNumber, next.Deposited,
		next.Expected, next.MismatchReason, now, actor)
	return d, err
}

func (p payment) check(ctx context.Context, q Querier, sale *SaleRef, actor uuid.UUID, now time.Time) ([]apperr.FieldError, error) {
	r, err := p.read(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	expected, err := p.expected(ctx, q, sale.ID)
	if err != nil {
		return nil, err
	}
	var problems []apperr.FieldError
	problems = required(problems, "bank", "El banco", r.Bank != nil)
	problems = required(problems, "paymentDate", "La fecha de la operación", r.PaymentDate != nil)
	problems = required(problems, "transactionNumber", "El número de operación", r.TransactionNumber != nil)
	problems = required(problems, "depositedAmount", "El monto depositado", r.Deposited != nil)
	if differs(r.Deposited, expected) && r.MismatchReason == nil {
		problems = append(problems, apperr.FieldError{
			Field: "mismatchReason", Code: "MISMATCH_REASON_REQUIRED",
			Message: "El monto no coincide con " + p.expectedLabel + ": escriba el motivo de la diferencia.",
		})
	}
	// The expected amount stored with the row is exactly what this check compared.
	if len(problems) == 0 && !equalText(r.Expected, expected) {
		_, err = q.Exec(ctx, `UPDATE sale_payments SET expected_amount = $3::text::numeric, updated_by = $4, updated_at = $5,
			row_version = row_version + 1 WHERE sale_id = $1 AND payment_kind = $2::text::payment_kind`,
			sale.ID, p.kind, expected, actor, now)
		if err != nil {
			return nil, err
		}
	}
	return problems, nil
}

// reconcileMismatch applies the mismatch rule: a deposited amount that equals the expected one needs no
// reason (a stale one is dropped), and one that differs must carry a reason.
func reconcileMismatch(d *draft, field string, amount, expected, reason *string, expectedLabel string) *string {
	switch {
	case amount == nil || expected == nil:
		return reason
	case !differs(amount, expected):
		return nil
	case reason == nil:
		d.fail(field, "MISMATCH_REASON_REQUIRED",
			"El monto no coincide con "+expectedLabel+": escriba el motivo de la diferencia.")
	}
	return reason
}
