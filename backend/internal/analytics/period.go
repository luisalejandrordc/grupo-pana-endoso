package analytics

import (
	"endosoflow/internal/platform/apperr"
	"time"
)

// lima is Peru's fixed offset; the business works in Lima dates and Peru has no daylight saving time.
var lima = time.FixedZone("Lima", -5*3600)

const (
	dateLayout      = "2006-01-02"
	defaultDays     = 90
	maxPeriodDays   = 731
	granularityDay  = "day"
	granularityWeek = "week"
	granularityMon  = "month"
)

// Period is the range a response covers plus the equal-length range right before it.
type Period struct {
	From, To         time.Time // first and last Lima day, inclusive (midnight UTC, used as dates)
	PrevFrom, PrevTo time.Time // the same for the previous period
	Granularity      string
}

// ResolvePeriod applies the defaults (the last 90 days ending today) and validates the range.
// from and to are Lima dates; nil means "use the default".
func ResolvePeriod(from, to *time.Time, now time.Time) (Period, error) {
	today := dateOnly(now.In(lima))
	end := today
	if to != nil {
		end = dateOnly(*to)
	}
	start := end.AddDate(0, 0, -(defaultDays - 1))
	if from != nil {
		start = dateOnly(*from)
	}
	if start.After(end) {
		return Period{}, apperr.BadRequest("INVALID_QUERY", "La fecha inicial no puede ser posterior a la final.")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > maxPeriodDays {
		return Period{}, apperr.BadRequest("INVALID_QUERY", "El periodo no puede superar los dos años.")
	}
	prevEnd := start.AddDate(0, 0, -1)
	p := Period{
		From: start, To: end,
		PrevFrom: prevEnd.AddDate(0, 0, -(days - 1)), PrevTo: prevEnd,
	}
	switch {
	case days <= 31:
		p.Granularity = granularityDay
	case days <= 180:
		p.Granularity = granularityWeek
	default:
		p.Granularity = granularityMon
	}
	return p, nil
}

// dateOnly keeps the calendar date of t and drops the time and zone, so it can travel as a SQL date.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// limaStart is the instant at which the Lima day d begins.
func limaStart(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, lima)
}

// Bounds are the period as half-open instants: [FromTS, ToTS) and [PrevFromTS, PrevToTS).
func (p Period) bounds() (fromTS, toTS, prevFromTS, prevToTS time.Time) {
	return limaStart(p.From), limaStart(p.To.AddDate(0, 0, 1)), limaStart(p.PrevFrom), limaStart(p.PrevTo.AddDate(0, 0, 1))
}
