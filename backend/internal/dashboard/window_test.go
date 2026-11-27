package dashboard

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDefaultWindowIsTheLast90DaysInLimaTime(t *testing.T) {
	// 03:00 UTC on the 23rd is still the evening of the 22nd in Lima.
	now := time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC)
	w, err := resolveWindow(Filter{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.period(); got.From != "2026-08-25" || got.To != "2026-11-22" {
		t.Fatalf("period = %+v", got)
	}
	// The window spans whole Lima days: from 00:00 of the first to 00:00 after the last.
	if !w.start.Equal(time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC)) || !w.end.Equal(time.Date(2026, 11, 23, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("instants = %v .. %v", w.start, w.end)
	}
	if got := w.months(); len(got) != 4 || got[0] != "2026-08" || got[3] != "2026-11" {
		t.Fatalf("months = %v", got)
	}
}

func TestWindowRejectsBadPeriods(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	for name, f := range map[string]Filter{
		"not a date":       {From: "22/11/2026"},
		"to not a date":    {To: "soon"},
		"from after to":    {From: "2026-11-10", To: "2026-11-01"},
		"over three years": {From: "2020-01-01", To: "2026-01-01"},
	} {
		if _, err := resolveWindow(f, now); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := resolveWindow(Filter{From: "2026-11-22", To: "2026-11-22"}, now); err != nil {
		t.Fatalf("a single day is a valid period: %v", err)
	}
}

func TestRateAndRounding(t *testing.T) {
	if rate(0, 0) != nil {
		t.Fatal("no completed and no cancelled sales means no rate")
	}
	if got := rate(1, 2); got == nil || *got != 33.3 {
		t.Fatalf("rate(1,2) = %v", got)
	}
	if got := rate(3, 0); got == nil || *got != 100 {
		t.Fatalf("rate(3,0) = %v", got)
	}
	v := 2.25
	if got := round1(&v); *got != 2.3 {
		t.Fatalf("round1(2.25) = %v", *got)
	}
}

func TestFilterWhereOnlyAddsWhatWasAsked(t *testing.T) {
	var b builder
	if got := (Filter{}).where(&b, "s"); got != "TRUE" || len(b.args) != 0 {
		t.Fatalf("empty filter = %q with %d args", got, len(b.args))
	}
	id, model := uuid.New(), uuid.New()
	b = builder{}
	got := Filter{AdvisorID: &id, ModelID: &model, Temperature: "hot", CustomerType: "embajada"}.where(&b, "s")
	if len(b.args) != 4 {
		t.Fatalf("args = %d, want 4", len(b.args))
	}
	for _, want := range []string{"s.assigned_advisor_id = $1", "s.vehicle_model_id", "sale_temperature", "customer_type"} {
		if !strings.Contains(got, want) {
			t.Errorf("where %q lacks %q", got, want)
		}
	}
}

func TestParseFilterRejectsUnknownValues(t *testing.T) {
	for _, q := range []string{"advisorId=nope", "modelId=1", "customerType=alien", "temperature=lukewarm"} {
		if _, err := parseFilter(httptest.NewRequest("GET", "/x?"+q, nil)); err == nil {
			t.Errorf("%s should be rejected", q)
		}
	}
	f, err := parseFilter(httptest.NewRequest("GET", "/x?from=2026-01-01&temperature=hot&customerType=embajada", nil))
	if err != nil || f.From != "2026-01-01" || f.Temperature != "hot" || f.CustomerType != "embajada" {
		t.Fatalf("got %+v, %v", f, err)
	}
}

func TestAttentionSortsOverdueFirstThenApproachingThenStalled(t *testing.T) {
	d := func(h int) *time.Time { v := time.Date(2026, 11, 22, h, 0, 0, 0, time.UTC); return &v }
	items := []AttentionItem{
		{SaleCode: "stalled-old", Kind: KindStalled, WaitingSince: d(1)},
		{SaleCode: "approaching", Kind: KindApproaching, DueDate: d(20)},
		{SaleCode: "overdue-recent", Kind: KindOverdue, DueDate: d(9)},
		{SaleCode: "overdue-old", Kind: KindOverdue, DueDate: d(2)},
	}
	sortAttention(items)
	var got []string
	for _, it := range items {
		got = append(got, it.SaleCode)
	}
	if strings.Join(got, ",") != "overdue-old,overdue-recent,approaching,stalled-old" {
		t.Fatalf("order = %v", got)
	}
}
