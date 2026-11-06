package tasks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseMoney(t *testing.T) {
	good := map[string]int64{"1": 100, "35000": 3500000, "35000.5": 3500050, "35000.50": 3500050, " 12.07 ": 1207, "0.01": 1}
	for in, want := range good {
		got, ok := parseMoney(in)
		if !ok || got != want {
			t.Errorf("parseMoney(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "0.00", "-5", "1.234", "1,5", "abc", "12345678901", "1e3", ".5"} {
		if _, ok := parseMoney(in); ok {
			t.Errorf("parseMoney(%q) accepted", in)
		}
	}
}

func TestFormatCents(t *testing.T) {
	for cents, want := range map[int64]string{1: "0.01", 100: "1.00", 3500050: "35000.50"} {
		if got := formatCents(cents); got != want {
			t.Errorf("formatCents(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	if _, ok := parseDate("2026-02-28"); !ok {
		t.Error("valid date refused")
	}
	for _, in := range []string{"", "2026-02-30", "26-02-28", "2026/02/28", "1999-12-31", "2101-01-01", "hoy"} {
		if _, ok := parseDate(in); ok {
			t.Errorf("parseDate(%q) accepted", in)
		}
	}
}

func TestCompact(t *testing.T) {
	if got := compact("  ab 12\tc "); got != "AB12C" {
		t.Fatalf("compact = %q", got)
	}
}

func TestNormalizeSignerDocument(t *testing.T) {
	type c struct {
		docType, number string
		wantNumber      string
		ok              bool
	}
	for _, tc := range []c{
		{"dni", " 12345678 ", "12345678", true},
		{"DNI", "1234567", "", false},
		{"DNI", "1234567A", "", false},
		{"CE", "ab1234567", "AB1234567", true},
		{"CE", "12345678", "", false},
		{"PAS", "ab 12345", "AB12345", true},
		{"PAS", "AB1", "", false},
		{"CDI", "dip-1234", "", false},
		{"CDI", "dip12345", "DIP12345", true},
		{"RUC", "20123456789", "", false},
	} {
		_, number, ok := normalizeSignerDocument(tc.docType, tc.number)
		if ok != tc.ok || (ok && number != tc.wantNumber) {
			t.Errorf("%s %q: got %q, %v; want %q, %v", tc.docType, tc.number, number, ok, tc.wantNumber, tc.ok)
		}
	}
}

func testSale() *SaleRef {
	return &SaleRef{CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), CustomerType: "embajada"}
}

func field[T any](v T) Field[T] { return Field[T]{Set: true, Value: &v} }

func TestDraftMoney(t *testing.T) {
	d := newDraft(testSale())
	if v, ok := d.money(field(Amount("1500.5")), "x", "El monto"); !ok || v == nil || *v != "1500.50" {
		t.Fatalf("valid amount: %v %v", v, ok)
	}
	if v, ok := d.money(Field[Amount]{Set: true}, "x", "El monto"); !ok || v != nil {
		t.Fatal("null must clear the amount")
	}
	if _, ok := d.money(Field[Amount]{}, "x", "El monto"); ok {
		t.Fatal("an absent field must not be set")
	}
	if len(d.problems) != 0 {
		t.Fatalf("unexpected problems: %v", d.problems)
	}
	if _, ok := d.money(field(Amount("-3")), "x", "El monto"); ok || len(d.problems) != 1 || d.problems[0].Code != "INVALID_AMOUNT" {
		t.Fatalf("negative amount: ok=%v problems=%v", ok, d.problems)
	}
}

func TestDraftDateWarnsBeforeTheSale(t *testing.T) {
	d := newDraft(testSale())
	if v, ok := d.date(field("2026-03-11"), "d", "La fecha"); !ok || v == nil {
		t.Fatal("a later date must be accepted")
	}
	if len(d.warnings) != 0 {
		t.Fatalf("no warning expected: %v", d.warnings)
	}
	if v, ok := d.date(field("2026-01-05"), "d", "La fecha"); !ok || v == nil {
		t.Fatal("an earlier date is only a warning")
	}
	if len(d.warnings) != 1 || d.warnings[0].Code != "DATE_BEFORE_SALE" {
		t.Fatalf("warnings = %v", d.warnings)
	}
	if _, ok := d.date(field("2026-02-30"), "d", "La fecha"); ok || d.result() == nil {
		t.Fatal("an impossible date must block the save")
	}
}

func TestDraftCodeAndText(t *testing.T) {
	d := newDraft(testSale())
	if v, ok := d.code(field("bcp"), "bank", "El banco", allBanks...); !ok || *v != "bcp" {
		t.Fatal("valid code refused")
	}
	if _, ok := d.code(field("otro"), "bank", "El banco", allBanks...); ok {
		t.Fatal("unknown code accepted")
	}
	if v, ok := d.text(field("  hola  "), "t", "El texto", 10); !ok || *v != "hola" {
		t.Fatal("text must be trimmed")
	}
	if v, ok := d.text(field("   "), "t", "El texto", 10); !ok || v != nil {
		t.Fatal("blank text must clear the field")
	}
	if _, ok := d.text(field("12345678901"), "t", "El texto", 10); ok {
		t.Fatal("too long text accepted")
	}
}

func TestDraftIdentifier(t *testing.T) {
	d := newDraft(testSale())
	if v, ok := d.identifier(field(" jtd-1234 567 "), "s", "El serie", IdentSeries); !ok || *v != "JTD-1234567" {
		t.Fatalf("series: %v %v", v, ok)
	}
	if v, ok := d.identifier(field("jtdkb20u973123456"), "v", "El VIN", IdentVIN); !ok || *v != "JTDKB20U973123456" {
		t.Fatalf("vin: %v %v", v, ok)
	}
	if _, ok := d.identifier(field("JTDKB20U97312345I"), "v", "El VIN", IdentVIN); ok {
		t.Fatal("a VIN with the letter I must be refused")
	}
	if _, ok := d.identifier(field("JTDKB20U9731234"), "v", "El VIN", IdentVIN); ok {
		t.Fatal("a short VIN must be refused")
	}
	if v, ok := d.identifier(field("abc123"), "p", "La placa", IdentPlate); !ok || *v != "ABC123" {
		t.Fatalf("plate: %v %v", v, ok)
	}
	if _, ok := d.identifier(field("AB-123"), "p", "La placa", IdentPlate); ok {
		t.Fatal("a plate with a hyphen must be refused")
	}
}

func TestDraftNoteIgnoresUnchangedAndHidesPersonalData(t *testing.T) {
	d := newDraft(testSale())
	a, b := "x", "y"
	d.note("same", &a, &a, false)
	d.note("amount", &a, &b, false)
	d.note("signerFirstName", &a, &b, true)
	if _, ok := d.changes["same"]; ok {
		t.Error("an unchanged field must not be recorded")
	}
	if d.changes["signerFirstName"] != "changed" {
		t.Errorf("personal data must be recorded as changed, got %v", d.changes["signerFirstName"])
	}
	if _, ok := d.changes["amount"]; !ok {
		t.Error("a business field must be recorded")
	}
}

func TestReconcileMismatch(t *testing.T) {
	str := func(s string) *string { return &s }
	t.Run("equal amounts drop a stale reason", func(t *testing.T) {
		d := newDraft(testSale())
		if got := reconcileMismatch(d, "r", str("100.00"), str("100.00"), str("old"), "el esperado"); got != nil || d.result() != nil {
			t.Fatalf("got %v, problems %v", got, d.problems)
		}
	})
	t.Run("different amounts need a reason", func(t *testing.T) {
		d := newDraft(testSale())
		reconcileMismatch(d, "r", str("99.00"), str("100.00"), nil, "el esperado")
		if d.result() == nil || d.problems[0].Code != "MISMATCH_REASON_REQUIRED" {
			t.Fatalf("problems = %v", d.problems)
		}
	})
	t.Run("different amounts keep their reason", func(t *testing.T) {
		d := newDraft(testSale())
		if got := reconcileMismatch(d, "r", str("99.00"), str("100.00"), str("banco"), "el esperado"); got == nil || *got != "banco" || d.result() != nil {
			t.Fatalf("got %v, problems %v", got, d.problems)
		}
	})
	t.Run("unknown expected amount is not compared", func(t *testing.T) {
		d := newDraft(testSale())
		reconcileMismatch(d, "r", str("99.00"), nil, nil, "el esperado")
		if d.result() != nil {
			t.Fatalf("problems = %v", d.problems)
		}
	})
}

func TestDiffers(t *testing.T) {
	a, b, c := "100.00", "100.0", "100.01"
	if differs(&a, &b) {
		t.Error("100.00 and 100.0 are the same amount")
	}
	if !differs(&a, &c) {
		t.Error("100.00 and 100.01 differ")
	}
	if differs(nil, &a) || differs(&a, nil) {
		t.Error("an unknown amount never differs")
	}
}

func TestFieldDistinguishesAbsentNullAndValue(t *testing.T) {
	var in struct {
		A Field[string] `json:"a"`
		B Field[string] `json:"b"`
		C Field[string] `json:"c"`
		D Field[Amount] `json:"d"`
		E Field[Amount] `json:"e"`
	}
	if err := json.Unmarshal([]byte(`{"a": null, "b": "x", "d": 1500.5, "e": "20"}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.A.Set || in.A.Value != nil {
		t.Errorf("null: %+v", in.A)
	}
	if !in.B.Set || in.B.Value == nil || *in.B.Value != "x" {
		t.Errorf("value: %+v", in.B)
	}
	if in.C.Set {
		t.Errorf("absent: %+v", in.C)
	}
	if !in.D.Set || *in.D.Value != "1500.5" || *in.E.Value != "20" {
		t.Errorf("amounts: %+v %+v", in.D, in.E)
	}
}

func TestDecodeStrictRejectsUnknownKeys(t *testing.T) {
	var in task3Patch
	if err := decodeStrict(json.RawMessage(`{"vin": "x", "surprise": 1}`), &in); err == nil {
		t.Fatal("an unknown key must be refused")
	}
	if err := decodeStrict(nil, &in); err != nil {
		t.Fatalf("an empty body is an empty patch: %v", err)
	}
}

func testNow() time.Time { return time.Date(2026, 3, 12, 15, 0, 0, 0, time.UTC) }
