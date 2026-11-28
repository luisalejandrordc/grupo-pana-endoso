package analytics

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// querier is what the queries need from a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// scope is everything a query needs: the period, who may be seen and the filters.
type scope struct {
	period       Period
	advisor      *uuid.UUID
	customerType *string
	model        *uuid.UUID
	temperature  *string
	now          time.Time
	own          bool // the caller is an Advisor and sees only their own sales
}

// args returns the thirteen placeholders that the p CTE declares. Every query starts with cteP, so every
// query takes exactly these arguments, used or not.
func (s scope) args() []any {
	fromTS, toTS, prevFromTS, prevToTS := s.period.bounds()
	var advisor, model any
	if s.advisor != nil {
		advisor = *s.advisor
	}
	if s.model != nil {
		model = *s.model
	}
	var customerType, temperature any
	if s.customerType != nil {
		customerType = *s.customerType
	}
	if s.temperature != nil {
		temperature = *s.temperature
	}
	return []any{
		fromTS, toTS, prevFromTS, prevToTS,
		advisor, customerType, model, temperature, s.now,
		s.period.From, s.period.To, s.period.PrevFrom, s.period.PrevTo,
	}
}

// cteP names the placeholders once so the rest of the SQL reads p.from_ts instead of $1.
const cteP = `WITH p AS (SELECT
	$1::timestamptz AS from_ts, $2::timestamptz AS to_ts,
	$3::timestamptz AS prev_from_ts, $4::timestamptz AS prev_to_ts,
	$5::uuid AS advisor_id, $6::text AS customer_type, $7::uuid AS model_id, $8::text AS temperature,
	$9::timestamptz AS now_ts,
	$10::date AS from_date, $11::date AS to_date, $12::date AS prev_from_date, $13::date AS prev_to_date)`

// cteFS is the set of sales the caller may see and asked for, with their money. value is the quoted final
// price, or the catalog reference price taken at creation while no quote exists yet.
const cteFS = `, fs AS (
	SELECT s.*, f.quoted_final_price AS quoted, f.initial_commission, f.final_commission,
		f.final_toyota_amount, f.accessory_amount AS accessories, f.release_processing_amount AS release_processing,
		coalesce(f.quoted_final_price, s.vehicle_reference_price_snapshot) AS value
	FROM sale_registers s
	LEFT JOIN sale_financials f ON f.sale_id = s.id
	CROSS JOIN p
	WHERE (p.advisor_id IS NULL OR s.assigned_advisor_id = p.advisor_id)
	  AND (p.customer_type IS NULL OR s.customer_type_snapshot::text = p.customer_type)
	  AND (p.model_id IS NULL OR s.vehicle_model_id = p.model_id)
	  AND (p.temperature IS NULL OR s.sale_temperature::text = p.temperature))`

// cteCur is the current task of every active sale: the one in progress, or else the first unfinished one.
const cteCur = `, cur AS (
	SELECT fs.id AS sale_id, fs.assigned_advisor_id AS advisor_id, t.task_number, t.status::text AS status,
		t.due_date, d.milestone_number, d.recommended_duration_hours AS dur, el.eligible_at
	FROM fs
	JOIN LATERAL (
		SELECT x.id, x.task_number, x.status, x.due_date FROM task_instances x
		WHERE x.sale_id = fs.id AND x.status <> 'completed'
		ORDER BY (x.status = 'in_progress') DESC, x.task_number
		LIMIT 1
	) t ON true
	JOIN task_definitions d ON d.task_number = t.task_number
	LEFT JOIN v_task_eligibility el ON el.task_instance_id = t.id
	WHERE fs.overall_state = 'active')`

// cteCl classifies each current task. overdue: past its due date. approaching: past 80% of its duration.
// idle: nothing is in progress and the task has been eligible for more than 24 hours. The rest are on time.
const cteCl = `, cl AS (
	SELECT c.*, CASE
		WHEN c.status = 'in_progress' AND c.due_date IS NOT NULL AND c.due_date < p.now_ts THEN 'overdue'
		WHEN c.status = 'in_progress' AND c.due_date IS NOT NULL AND c.dur IS NOT NULL
			AND p.now_ts >= c.due_date - (c.dur * 0.2)::float8 * interval '1 hour' THEN 'approaching'
		WHEN c.status = 'pending' AND c.eligible_at IS NOT NULL
			AND c.eligible_at < p.now_ts - interval '24 hours' THEN 'idle'
		ELSE 'on_time' END AS kind
	FROM cur c CROSS JOIN p)`

// money renders a SUM-like expression as decimal text with two decimals; null sums become 0.00.
func money(expr string) string { return "coalesce(" + expr + ", 0)::numeric(14,2)::text" }

// inPeriod and inPrevious test a timestamp column against the period and the previous period.
func inPeriod(col string) string { return fmt.Sprintf("(%s >= p.from_ts AND %s < p.to_ts)", col, col) }

func inPrevious(col string) string {
	return fmt.Sprintf("(%s >= p.prev_from_ts AND %s < p.prev_to_ts)", col, col)
}

// onDates and onPrevDates do the same for date columns.
func onDates(col string) string { return fmt.Sprintf("(%s BETWEEN p.from_date AND p.to_date)", col) }

func onPrevDates(col string) string {
	return fmt.Sprintf("(%s BETWEEN p.prev_from_date AND p.prev_to_date)", col)
}

// bucketCTE lists the first day of every bucket of the period. granularity comes from a fixed set.
func bucketCTE(granularity string) (string, error) {
	switch granularity {
	case granularityDay, granularityWeek, granularityMon:
		return fmt.Sprintf(`, b AS (SELECT gs::date AS bucket FROM p,
	generate_series(date_trunc('%[1]s', p.from_date::timestamp), p.to_date::timestamp, interval '1 %[1]s') AS gs)`, granularity), nil
	}
	return "", apperr.Internal(fmt.Errorf("unknown granularity %q", granularity))
}

// bucketOf is the first day of the bucket that holds the timestamp column, in Lima time.
func bucketOf(granularity, col string) string {
	return fmt.Sprintf("date_trunc('%s', %s AT TIME ZONE 'America/Lima')::date", granularity, col)
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den)
	return &v
}

func mkRatio(num, den int) Ratio {
	return Ratio{Numerator: num, Denominator: den, Value: ratio(num, den)}
}
