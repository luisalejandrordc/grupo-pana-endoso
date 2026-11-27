package dashboard

import (
	"endosoflow/internal/platform/apperr"
	"strconv"
	"strings"
	"time"
)

const (
	dateLayout = "2006-01-02"
	// defaultDays is the period when the caller picks none: the last 90 days, today included.
	defaultDays = 90
	// maxDays keeps a single request from scanning years of history.
	maxDays = 1100
	// stalledAfter is how long a task may be able to start before the dashboard calls it stalled.
	stalledAfter = 48 * time.Hour
)

// limaZone is fixed because Peru has no daylight saving time.
var limaZone = time.FixedZone("America/Lima", -5*60*60)

// limaSQL is the same zone as a Postgres interval, for converting a timestamp to Lima's wall clock.
const limaSQL = "INTERVAL '-05:00'"

// window is the period of a request: the two calendar dates (Lima, both included) and the instants they span.
type window struct {
	from, to   time.Time // midnight Lima of the first and last day
	start, end time.Time // [start, end): the first instant of from and the first instant after to
}

func (w window) period() Period {
	return Period{From: w.from.Format(dateLayout), To: w.to.Format(dateLayout)}
}

var errInvalidPeriod = apperr.BadRequest("INVALID_QUERY", "El periodo no es válido.")

// resolveWindow turns the optional dates of a filter into a window. A missing "to" is today and a missing
// "from" is 89 days before "to".
func resolveWindow(f Filter, now time.Time) (window, error) {
	local := now.In(limaZone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, limaZone)

	to := today
	if f.To != "" {
		t, err := time.ParseInLocation(dateLayout, f.To, limaZone)
		if err != nil {
			return window{}, errInvalidPeriod
		}
		to = t
	}
	from := to.AddDate(0, 0, -(defaultDays - 1))
	if f.From != "" {
		t, err := time.ParseInLocation(dateLayout, f.From, limaZone)
		if err != nil {
			return window{}, errInvalidPeriod
		}
		from = t
	}
	if from.After(to) {
		return window{}, errInvalidPeriod
	}
	if to.Sub(from) > time.Duration(maxDays)*24*time.Hour {
		return window{}, apperr.BadRequest("INVALID_QUERY", "El periodo no puede superar los 3 años.")
	}
	return window{from: from, to: to, start: from, end: to.AddDate(0, 0, 1)}, nil
}

// months lists every month (YYYY-MM) the window touches.
func (w window) months() []string {
	var out []string
	m := time.Date(w.from.Year(), w.from.Month(), 1, 0, 0, 0, 0, limaZone)
	for !m.After(w.to) {
		out = append(out, m.Format("2006-01"))
		m = m.AddDate(0, 1, 0)
	}
	return out
}

// builder collects the arguments of one query and hands out their placeholders. Every value reaches the
// database as an argument; only fixed fragments written in this package are ever concatenated into the SQL.
type builder struct{ args []any }

func (b *builder) add(v any) string {
	b.args = append(b.args, v)
	return "$" + strconv.Itoa(len(b.args))
}

// ts adds an instant.
func (b *builder) ts(t time.Time) string { return b.add(t) + "::timestamptz" }

// date adds a calendar date. It travels as text and is cast in SQL, so the driver never has to guess.
func (b *builder) date(t time.Time) string { return b.add(t.Format(dateLayout)) + "::text::date" }

// where builds the conditions that pick the sales, on the sale table known as alias. Every dashboard query
// that touches sales uses it, so the filters and the Advisor scope mean the same thing everywhere.
func (f Filter) where(b *builder, alias string) string {
	parts := []string{"TRUE"}
	if f.AdvisorID != nil {
		parts = append(parts, alias+".assigned_advisor_id = "+b.add(*f.AdvisorID))
	}
	if f.CustomerType != "" {
		parts = append(parts, alias+".customer_type_snapshot = "+b.add(f.CustomerType)+"::text::customer_type")
	}
	if f.Temperature != "" {
		parts = append(parts, alias+".sale_temperature = "+b.add(f.Temperature)+"::text::sale_temperature")
	}
	if f.ModelID != nil {
		parts = append(parts, alias+".vehicle_model_id = "+b.add(*f.ModelID))
	}
	if f.VersionID != nil {
		parts = append(parts, alias+".vehicle_version_id = "+b.add(*f.VersionID))
	}
	return strings.Join(parts, " AND ")
}

// sumRaw is a SQL sum of an amount over the rows that match a condition, 0 when none do.
func sumRaw(expr, cond string) string {
	return "coalesce(sum(" + expr + ") FILTER (WHERE " + cond + "), 0)"
}

// asMoney renders an amount expression as a two-decimal string.
func asMoney(expr string) string { return "(" + expr + ")::numeric(14,2)::text" }

// money is the sum of an amount, as a two-decimal string.
func money(expr, cond string) string { return asMoney(sumRaw(expr, cond)) }

// round1 rounds to one decimal, the precision every rate and duration is shown with.
func round1(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := float64(int64(*v*10+signHalf(*v))) / 10
	return &r
}

func signHalf(v float64) float64 {
	if v < 0 {
		return -0.5
	}
	return 0.5
}

// rate is part / (part + other) as a percentage with one decimal; null when both are zero.
func rate(part, other int) *float64 {
	total := part + other
	if total == 0 {
		return nil
	}
	v := float64(part) * 100 / float64(total)
	return round1(&v)
}
