package tasks

import (
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	moneyPattern   = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)
	vinPattern     = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
	platePattern   = regexp.MustCompile(`^[A-Z0-9]{6}$`)
	invoicePattern = regexp.MustCompile(`^[A-Z0-9]{4}-[0-9]{1,8}$`)
	dniPattern     = regexp.MustCompile(`^[0-9]{8}$`)
	cePattern      = regexp.MustCompile(`^[A-Z0-9]{9}$`)
	pasPattern     = regexp.MustCompile(`^[A-Z0-9]{6,12}$`)
	cdiPattern     = regexp.MustCompile(`^[A-Z0-9]{8,12}$`)
)

// parseMoney turns "35000", "35000.5" or "35000.50" into cents. It rejects zero, negatives, more than two
// decimals and anything that would not fit numeric(12,2).
func parseMoney(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if !moneyPattern.MatchString(raw) {
		return 0, false
	}
	whole, frac, _ := strings.Cut(raw, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false
	}
	total := units*100 + cents
	return total, total > 0
}

func formatCents(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

// centsOf reads an amount the database returned as text. A missing value reports false.
func centsOf(text *string) (int64, bool) {
	if text == nil {
		return 0, false
	}
	return parseMoney(*text)
}

// parseDate accepts a calendar date written YYYY-MM-DD. Impossible dates (February 30) and years outside
// 2000 to 2100 are refused.
func parseDate(raw string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil || t.Year() < 2000 || t.Year() > 2100 {
		return time.Time{}, false
	}
	return t, true
}

// compact upper-cases a value and removes every space inside it (series, DAM and transaction numbers).
func compact(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeSignerDocument upper-cases and checks the signer's document. A RUC is never a signer.
func normalizeSignerDocument(docType, docNumber string) (string, string, bool) {
	docType = strings.ToUpper(strings.TrimSpace(docType))
	docNumber = compact(docNumber)
	var pattern *regexp.Regexp
	switch docType {
	case "DNI":
		pattern = dniPattern
	case "CE":
		pattern = cePattern
	case "PAS":
		pattern = pasPattern
	case "CDI":
		pattern = cdiPattern
	default:
		return "", "", false
	}
	return docType, docNumber, pattern.MatchString(docNumber)
}

func oneOf(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// draft collects what one data save finds: field errors that block the save, warnings that do not, and
// the audit summary of what changed.
type draft struct {
	sale     *SaleRef
	problems []apperr.FieldError
	warnings []Warning
	changes  map[string]any
}

func newDraft(sale *SaleRef) *draft {
	return &draft{sale: sale, changes: map[string]any{}}
}

func (d *draft) fail(field, code, message string) {
	d.problems = append(d.problems, apperr.FieldError{Field: field, Code: code, Message: message})
}

func (d *draft) warn(field, code, message string) {
	d.warnings = append(d.warnings, Warning{Field: field, Code: code, Message: message})
}

// result is the validation error to return, or nil when nothing blocks the save.
func (d *draft) result() *apperr.Error {
	if len(d.problems) == 0 {
		return nil
	}
	return apperr.Unprocessable("VALIDATION_FAILED", "Revise los datos ingresados.").WithFields(d.problems...)
}

// note records a changed value. Personal data is recorded as "changed" without its values.
func (d *draft) note(field string, before, after *string, personal bool) {
	if equalText(before, after) {
		return
	}
	if personal {
		d.changes[field] = "changed"
		return
	}
	d.changes[field] = map[string]*string{"before": before, "after": after}
}

func equalText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// text validates a free-text field. It returns the cleaned value (nil clears the field) and whether the
// client sent the field at all.
func (d *draft) text(f Field[string], field, label string, max int) (*string, bool) {
	if !f.Set {
		return nil, false
	}
	if f.Value == nil {
		return nil, true
	}
	value := strings.TrimSpace(*f.Value)
	if value == "" {
		return nil, true
	}
	if utf8.RuneCountInString(value) > max {
		d.fail(field, "TOO_LONG", fmt.Sprintf("%s es demasiado largo (máximo %d caracteres).", label, max))
		return nil, false
	}
	return &value, true
}

// code validates a value that must be one of a fixed list (an enum value).
func (d *draft) code(f Field[string], field, label string, allowed ...string) (*string, bool) {
	if !f.Set {
		return nil, false
	}
	if f.Value == nil || strings.TrimSpace(*f.Value) == "" {
		return nil, true
	}
	value := strings.TrimSpace(*f.Value)
	if !oneOf(value, allowed...) {
		d.fail(field, "INVALID_VALUE", fmt.Sprintf("%s no es válido.", label))
		return nil, false
	}
	return &value, true
}

// money validates an amount and returns it as a decimal string such as "1500.00".
func (d *draft) money(f Field[Amount], field, label string) (*string, bool) {
	if !f.Set {
		return nil, false
	}
	if f.Value == nil || strings.TrimSpace(string(*f.Value)) == "" {
		return nil, true
	}
	cents, ok := parseMoney(string(*f.Value))
	if !ok {
		d.fail(field, "INVALID_AMOUNT", fmt.Sprintf("%s debe ser mayor que cero, con hasta dos decimales.", label))
		return nil, false
	}
	text := formatCents(cents)
	return &text, true
}

// date validates a calendar date. A date earlier than the day the sale was created only produces a warning.
func (d *draft) date(f Field[string], field, label string) (*string, bool) {
	if !f.Set {
		return nil, false
	}
	if f.Value == nil || strings.TrimSpace(*f.Value) == "" {
		return nil, true
	}
	t, ok := parseDate(*f.Value)
	if !ok {
		d.fail(field, "INVALID_DATE", fmt.Sprintf("%s no es una fecha válida (use el formato AAAA-MM-DD).", label))
		return nil, false
	}
	text := t.Format("2006-01-02")
	created := d.sale.CreatedAt.UTC().Truncate(24 * time.Hour)
	if t.Before(created.AddDate(0, 0, -1)) {
		d.warn(field, "DATE_BEFORE_SALE", fmt.Sprintf("%s es anterior a la creación de la venta.", label))
	}
	return &text, true
}

// decodeStrict reads a task's data body, refusing keys the task does not have.
func decodeStrict(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return apperr.BadRequest("INVALID_JSON", "Los datos de la tarea no son válidos.").WithCause(err)
	}
	return nil
}

// required adds a REQUIRED problem for a missing value. It is how completion checks report what is empty.
func required(problems []apperr.FieldError, field, label string, present bool) []apperr.FieldError {
	if present {
		return problems
	}
	return append(problems, apperr.FieldError{Field: field, Code: "REQUIRED", Message: label + " es obligatorio."})
}
