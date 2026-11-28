package analytics

import (
	"net/http/httptest"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestResolvePeriodDefaultsToTheLastNinetyDays(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, lima)
	p, err := ResolvePeriod(nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.To.Equal(day(2026, 11, 22)) || !p.From.Equal(day(2026, 8, 25)) {
		t.Fatalf("period = %s to %s", p.From.Format(dateLayout), p.To.Format(dateLayout))
	}
	if p.Granularity != granularityWeek {
		t.Fatalf("granularity = %s, want week", p.Granularity)
	}
}

// A little after midnight UTC it is still the previous evening in Lima.
func TestResolvePeriodUsesTheLimaDay(t *testing.T) {
	now := time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC) // 22:00 on the 22nd in Lima
	p, err := ResolvePeriod(nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.To.Equal(day(2026, 11, 22)) {
		t.Fatalf("to = %s, want 2026-11-22", p.To.Format(dateLayout))
	}
}

func TestResolvePeriodPreviousPeriodHasTheSameLength(t *testing.T) {
	from, to := day(2026, 3, 1), day(2026, 3, 31)
	p, err := ResolvePeriod(&from, &to, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !p.PrevTo.Equal(day(2026, 2, 28)) || !p.PrevFrom.Equal(day(2026, 1, 29)) {
		t.Fatalf("previous = %s to %s", p.PrevFrom.Format(dateLayout), p.PrevTo.Format(dateLayout))
	}
	if p.Granularity != granularityDay {
		t.Fatalf("granularity = %s, want day", p.Granularity)
	}
}

func TestResolvePeriodGranularity(t *testing.T) {
	to := day(2026, 6, 30)
	cases := []struct {
		days int
		want string
	}{{1, granularityDay}, {31, granularityDay}, {32, granularityWeek}, {180, granularityWeek}, {181, granularityMon}, {731, granularityMon}}
	for _, c := range cases {
		from := to.AddDate(0, 0, -(c.days - 1))
		p, err := ResolvePeriod(&from, &to, time.Now())
		if err != nil {
			t.Fatalf("%d days: %v", c.days, err)
		}
		if p.Granularity != c.want {
			t.Errorf("%d days: granularity = %s, want %s", c.days, p.Granularity, c.want)
		}
	}
}

func TestResolvePeriodRejectsBadRanges(t *testing.T) {
	to := day(2026, 6, 30)
	after := day(2026, 7, 1)
	tooLong := to.AddDate(0, 0, -731) // 732 days
	for name, from := range map[string]time.Time{"from after to": after, "longer than two years": tooLong} {
		if _, err := ResolvePeriod(&from, &to, time.Now()); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestPeriodBoundsAreLimaMidnights(t *testing.T) {
	p, _ := ResolvePeriod(ptr(day(2026, 3, 1)), ptr(day(2026, 3, 1)), time.Now())
	from, to, _, _ := p.bounds()
	if got := from.UTC(); !got.Equal(time.Date(2026, 3, 1, 5, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %s", got)
	}
	if got := to.Sub(from); got != 24*time.Hour {
		t.Errorf("a one-day period spans %s", got)
	}
}

func ptr[T any](v T) *T { return &v }

func TestParseFilterRejectsInvalidValues(t *testing.T) {
	for _, q := range []string{"from=2026-13-01", "to=hoy", "advisorId=nope", "modelId=1", "customerType=otro", "temperature=tibia", "groupBy=size"} {
		if _, err := parseFilter(httptest.NewRequest("GET", "/analytics/overview?"+q, nil)); err == nil {
			t.Errorf("%s: expected an error", q)
		}
	}
}

func TestParseFilterAcceptsValidValues(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/commercial?from=2026-01-01&to=2026-03-31&customerType=embajada&temperature=hot&groupBy=color&advisorId=7b0d1c5e-0a64-4c5d-9d57-0c0b6f0f9a11", nil)
	f, err := parseFilter(r)
	if err != nil {
		t.Fatal(err)
	}
	if f.From == nil || f.To == nil || f.AdvisorID == nil || *f.CustomerType != "embajada" || *f.Temperature != "hot" || f.GroupBy != "color" {
		t.Fatalf("unexpected filter %+v", f)
	}
}

func TestRatioIsNullWithoutADenominator(t *testing.T) {
	if ratio(0, 0) != nil {
		t.Fatal("0/0 must be null, not NaN")
	}
	if v := ratio(1, 4); v == nil || *v != 0.25 {
		t.Fatalf("1/4 = %v", v)
	}
}

func TestBucketCTERejectsUnknownGranularity(t *testing.T) {
	if _, err := bucketCTE("year'; DROP TABLE x; --"); err == nil {
		t.Fatal("a granularity outside the fixed set must be refused")
	}
}
