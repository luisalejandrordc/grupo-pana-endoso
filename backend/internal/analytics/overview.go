package analytics

import (
	"context"
	"endosoflow/internal/auth"
	"time"
)

// Overview answers the summary tab.
func (s *Service) Overview(ctx context.Context, actor auth.Actor, f Filter) (*Overview, error) {
	sc, err := s.scopeFor(actor, f, true)
	if err != nil {
		return nil, err
	}
	out := &Overview{Meta: s.meta(sc)}
	err = s.read(ctx, func(q querier) error {
		var err error
		if out.Snapshot, err = loadSnapshot(ctx, q, sc); err != nil {
			return err
		}
		if out.Period, err = loadOverviewPeriod(ctx, q, sc); err != nil {
			return err
		}
		if out.Money, err = loadOverviewMoney(ctx, q, sc, seesInternalMoney(actor)); err != nil {
			return err
		}
		if out.Pipeline, err = loadPipeline(ctx, q, sc); err != nil {
			return err
		}
		if out.Trend, err = loadTrend(ctx, q, sc); err != nil {
			return err
		}
		out.Attention.Total = out.Snapshot.Overdue + out.Snapshot.Approaching + out.Snapshot.Idle
		out.Attention.Items, err = loadAttention(ctx, q, sc)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func loadSnapshot(ctx context.Context, q querier, sc scope) (Snapshot, error) {
	var out Snapshot
	err := q.QueryRow(ctx, cteP+cteFS+cteCur+cteCl+`
SELECT
	(SELECT count(*) FROM fs WHERE overall_state = 'active')::int,
	count(*) FILTER (WHERE kind = 'overdue')::int,
	count(*) FILTER (WHERE kind = 'approaching')::int,
	count(*) FILTER (WHERE kind = 'idle')::int
FROM cl`, sc.args()...).Scan(&out.ActiveSales, &out.Overdue, &out.Approaching, &out.Idle)
	return out, err
}

func loadOverviewPeriod(ctx context.Context, q querier, sc scope) (OverviewPeriod, error) {
	var out OverviewPeriod
	var created, completed, cancelled, prevCreated, prevCompleted, prevCancelled int
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT
	count(*) FILTER (WHERE `+inPeriod("fs.created_at")+`)::int,
	count(*) FILTER (WHERE `+inPrevious("fs.created_at")+`)::int,
	count(*) FILTER (WHERE `+inPeriod("fs.completed_at")+`)::int,
	count(*) FILTER (WHERE `+inPrevious("fs.completed_at")+`)::int,
	count(*) FILTER (WHERE `+inPeriod("fs.cancelled_at")+`)::int,
	count(*) FILTER (WHERE `+inPrevious("fs.cancelled_at")+`)::int
FROM fs CROSS JOIN p`, sc.args()...).Scan(&created, &prevCreated, &completed, &prevCompleted, &cancelled, &prevCancelled)
	if err != nil {
		return out, err
	}
	out.Created = CountMetric{Value: created, Previous: prevCreated}
	out.Completed = CountMetric{Value: completed, Previous: prevCompleted}
	out.Cancelled = CountMetric{Value: cancelled, Previous: prevCancelled}
	out.CloseRate = RatioMetric{
		Numerator: completed, Denominator: completed + cancelled,
		Value: ratio(completed, completed+cancelled), Previous: ratio(prevCompleted, prevCompleted+prevCancelled),
	}
	cycle := `extract(epoch FROM fs.completed_at - fs.created_at) / 3600.0`
	err = q.QueryRow(ctx, cteP+cteFS+`
SELECT
	percentile_cont(0.5) WITHIN GROUP (ORDER BY `+cycle+`) FILTER (WHERE `+inPeriod("fs.completed_at")+`),
	percentile_cont(0.9) WITHIN GROUP (ORDER BY `+cycle+`) FILTER (WHERE `+inPeriod("fs.completed_at")+`),
	percentile_cont(0.5) WITHIN GROUP (ORDER BY `+cycle+`) FILTER (WHERE `+inPrevious("fs.completed_at")+`),
	count(*) FILTER (WHERE `+inPeriod("fs.completed_at")+`)::int
FROM fs CROSS JOIN p`, sc.args()...).Scan(&out.CycleTime.MedianHours, &out.CycleTime.P90Hours, &out.CycleTime.PreviousMedianHours, &out.CycleTime.SampleSize)
	return out, err
}

func loadOverviewMoney(ctx context.Context, q querier, sc scope, internal bool) (OverviewMoney, error) {
	var out OverviewMoney
	value := func(filter string) string { return money("sum(fs.value) FILTER (WHERE " + filter + ")") }
	active := "fs.overall_state = 'active'"
	completedNow := "fs.overall_state = 'completed' AND " + inPeriod("fs.completed_at")
	completedPrev := "fs.overall_state = 'completed' AND " + inPrevious("fs.completed_at")
	cancelledNow := "fs.overall_state = 'cancelled' AND " + inPeriod("fs.cancelled_at")
	avg := func(filter string) string {
		return "(avg(fs.value) FILTER (WHERE " + filter + "))::numeric(14,2)::text"
	}

	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT
	`+value(active)+`, count(*) FILTER (WHERE `+active+`)::int, count(*) FILTER (WHERE `+active+` AND fs.quoted IS NULL)::int,
	`+value(completedNow)+`, `+value(completedPrev)+`,
	`+avg(completedNow)+`, `+avg(completedPrev)+`,
	`+value(cancelledNow)+`, count(*) FILTER (WHERE `+cancelledNow+`)::int,
	count(*) FILTER (WHERE `+cancelledNow+` AND fs.quoted IS NULL)::int
FROM fs CROSS JOIN p`, sc.args()...).Scan(
		&out.PipelineValue.Amount, &out.PipelineValue.SaleCount, &out.PipelineValue.EstimatedCount,
		&out.CompletedValue.Value, &out.CompletedValue.Previous,
		&out.AverageTicket.Value, &out.AverageTicket.Previous,
		&out.LostValue.Amount, &out.LostValue.SaleCount, &out.LostValue.EstimatedCount)
	if err != nil {
		return out, err
	}

	err = q.QueryRow(ctx, cteP+cteFS+`
SELECT `+money("sum(fs.quoted)")+`, count(*)::int
FROM fs
LEFT JOIN task_instances t4 ON t4.sale_id = fs.id AND t4.task_number = 4
WHERE fs.overall_state = 'active' AND fs.quoted IS NOT NULL AND coalesce(t4.status::text, 'pending') <> 'completed'`,
		sc.args()...).Scan(&out.PendingCollection.Amount, &out.PendingCollection.SaleCount)
	if err != nil {
		return out, err
	}

	if out.Collected, err = loadPayments(ctx, q, sc, "pana"); err != nil {
		return out, err
	}
	if out.Invoiced, err = loadInvoiced(ctx, q, sc); err != nil {
		return out, err
	}
	if internal {
		in, err := loadInternalMoney(ctx, q, sc)
		if err != nil {
			return out, err
		}
		out.Internal = &in
	}
	return out, nil
}

// loadPayments sums deposited amounts of one payment kind by payment date, on sales that are not cancelled.
func loadPayments(ctx context.Context, q querier, sc scope, kind string) (MoneyMetric, error) {
	var out MoneyMetric
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT `+money("sum(pm.deposited_amount) FILTER (WHERE "+onDates("pm.payment_date")+")")+`,
	`+money("sum(pm.deposited_amount) FILTER (WHERE "+onPrevDates("pm.payment_date")+")")+`
FROM fs
JOIN sale_payments pm ON pm.sale_id = fs.id AND pm.payment_kind = '`+kind+`'
CROSS JOIN p
WHERE fs.overall_state <> 'cancelled'`, sc.args()...).Scan(&out.Value, &out.Previous)
	return out, err
}

func loadInvoiced(ctx context.Context, q querier, sc scope) (MoneyMetric, error) {
	var out MoneyMetric
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT `+money("sum(si.invoice_amount) FILTER (WHERE "+onDates("si.invoice_date")+")")+`,
	`+money("sum(si.invoice_amount) FILTER (WHERE "+onPrevDates("si.invoice_date")+")")+`
FROM fs
JOIN sale_invoices si ON si.sale_id = fs.id
CROSS JOIN p
WHERE fs.overall_state <> 'cancelled'`, sc.args()...).Scan(&out.Value, &out.Previous)
	return out, err
}

func loadInternalMoney(ctx context.Context, q querier, sc scope) (InternalMoney, error) {
	var out InternalMoney
	completedNow := "fs.overall_state = 'completed' AND " + inPeriod("fs.completed_at")
	completedPrev := "fs.overall_state = 'completed' AND " + inPrevious("fs.completed_at")
	rate := func(filter string) string {
		return "((sum(fs.final_commission) FILTER (WHERE " + filter + ") / nullif(sum(fs.quoted) FILTER (WHERE " + filter + "), 0)))::float8"
	}
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT
	`+money("sum(fs.final_commission) FILTER (WHERE "+completedNow+")")+`,
	`+money("sum(fs.final_commission) FILTER (WHERE "+completedPrev+")")+`,
	`+rate(completedNow)+`, `+rate(completedPrev)+`,
	`+money("sum(fs.initial_commission) FILTER (WHERE fs.overall_state = 'active')")+`,
	count(*) FILTER (WHERE fs.overall_state = 'active' AND fs.initial_commission IS NOT NULL)::int,
	`+money("sum(fs.final_commission - fs.initial_commission) FILTER (WHERE "+completedNow+")")+`
FROM fs CROSS JOIN p`, sc.args()...).Scan(
		&out.CommissionFinal.Value, &out.CommissionFinal.Previous,
		&out.CommissionRate.Value, &out.CommissionRate.Previous,
		&out.CommissionPipeline.Amount, &out.CommissionPipeline.SaleCount, &out.CommissionDrift)
	if err != nil {
		return out, err
	}
	if out.PaidToToyota, err = loadPayments(ctx, q, sc, "toyota"); err != nil {
		return out, err
	}
	err = q.QueryRow(ctx, cteP+cteFS+`
SELECT count(*)::int, `+money("sum(m.diff)")+` FROM (
	SELECT abs(pm.deposited_amount - pm.expected_amount) AS diff
	FROM fs JOIN sale_payments pm ON pm.sale_id = fs.id CROSS JOIN p
	WHERE fs.overall_state <> 'cancelled' AND `+onDates("pm.payment_date")+`
	  AND pm.deposited_amount IS NOT NULL AND pm.expected_amount IS NOT NULL
	  AND pm.deposited_amount <> pm.expected_amount
	UNION ALL
	SELECT abs(si.invoice_amount - si.expected_amount)
	FROM fs JOIN sale_invoices si ON si.sale_id = fs.id CROSS JOIN p
	WHERE fs.overall_state <> 'cancelled' AND `+onDates("si.invoice_date")+`
	  AND si.invoice_amount IS NOT NULL AND si.expected_amount IS NOT NULL
	  AND si.invoice_amount <> si.expected_amount
) m`, sc.args()...).Scan(&out.AmountMismatches.Count, &out.AmountMismatches.AbsoluteDifference)
	return out, err
}

func loadPipeline(ctx context.Context, q querier, sc scope) ([]MilestoneLoad, error) {
	rows, err := q.Query(ctx, cteP+cteFS+cteCur+cteCl+`
SELECT md.milestone_number, md.name_es,
	count(cl.sale_id)::int,
	count(cl.sale_id) FILTER (WHERE cl.kind = 'on_time')::int,
	count(cl.sale_id) FILTER (WHERE cl.kind = 'approaching')::int,
	count(cl.sale_id) FILTER (WHERE cl.kind = 'overdue')::int,
	count(cl.sale_id) FILTER (WHERE cl.kind = 'idle')::int,
	`+money("sum(fs.value)")+`
FROM milestone_definitions md
LEFT JOIN cl ON cl.milestone_number = md.milestone_number
LEFT JOIN fs ON fs.id = cl.sale_id
GROUP BY md.milestone_number, md.name_es
ORDER BY md.milestone_number`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MilestoneLoad{}
	for rows.Next() {
		var m MilestoneLoad
		if err := rows.Scan(&m.MilestoneNumber, &m.Name, &m.Active, &m.OnTime, &m.Approaching, &m.Overdue, &m.Idle, &m.Value); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadTrend(ctx context.Context, q querier, sc scope) ([]TrendBucket, error) {
	b, err := bucketCTE(sc.period.Granularity)
	if err != nil {
		return nil, err
	}
	g := sc.period.Granularity
	rows, err := q.Query(ctx, cteP+cteFS+b+`, ev AS (
	SELECT `+bucketOf(g, "fs.created_at")+` AS bucket, 1 AS created, 0 AS completed, 0 AS cancelled
		FROM fs CROSS JOIN p WHERE `+inPeriod("fs.created_at")+`
	UNION ALL
	SELECT `+bucketOf(g, "fs.completed_at")+`, 0, 1, 0
		FROM fs CROSS JOIN p WHERE `+inPeriod("fs.completed_at")+`
	UNION ALL
	SELECT `+bucketOf(g, "fs.cancelled_at")+`, 0, 0, 1
		FROM fs CROSS JOIN p WHERE `+inPeriod("fs.cancelled_at")+`)
SELECT b.bucket, coalesce(sum(ev.created), 0)::int, coalesce(sum(ev.completed), 0)::int, coalesce(sum(ev.cancelled), 0)::int
FROM b LEFT JOIN ev ON ev.bucket = b.bucket
GROUP BY b.bucket ORDER BY b.bucket`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrendBucket{}
	for rows.Next() {
		var bucket time.Time
		var t TrendBucket
		if err := rows.Scan(&bucket, &t.Created, &t.Completed, &t.Cancelled); err != nil {
			return nil, err
		}
		t.Bucket = bucket.Format(dateLayout)
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadAttention(ctx context.Context, q querier, sc scope) ([]AttentionItem, error) {
	rows, err := q.Query(ctx, cteP+cteFS+cteCur+cteCl+`
SELECT fs.id::text,
	'EF-' || to_char(fs.created_at AT TIME ZONE 'America/Lima', 'YYYY') || '-' || lpad(fs.sale_number::text, 5, '0'),
	fs.customer_legal_name_snapshot,
	fs.vehicle_model_name_snapshot || ' ' || fs.vehicle_version_name_snapshot,
	e.first_name || ' ' || e.last_name,
	cl.task_number, td.name_es, cl.kind, cl.due_date,
	CASE cl.kind
		WHEN 'overdue' THEN extract(epoch FROM p.now_ts - cl.due_date) / 3600.0
		WHEN 'approaching' THEN extract(epoch FROM cl.due_date - p.now_ts) / 3600.0
		ELSE extract(epoch FROM p.now_ts - cl.eligible_at) / 3600.0 END,
	fs.value::numeric(14,2)::text
FROM cl
JOIN fs ON fs.id = cl.sale_id
JOIN employees e ON e.id = fs.assigned_advisor_id
JOIN task_definitions td ON td.task_number = cl.task_number
CROSS JOIN p
WHERE cl.kind <> 'on_time'
ORDER BY CASE cl.kind WHEN 'overdue' THEN 0 WHEN 'approaching' THEN 1 ELSE 2 END,
	CASE cl.kind
		WHEN 'approaching' THEN extract(epoch FROM cl.due_date - p.now_ts)
		WHEN 'overdue' THEN extract(epoch FROM cl.due_date - p.now_ts)
		ELSE extract(epoch FROM cl.eligible_at - p.now_ts) END,
	fs.sale_number
LIMIT 10`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttentionItem{}
	for rows.Next() {
		var it AttentionItem
		if err := rows.Scan(&it.SaleID, &it.Code, &it.CustomerName, &it.Vehicle, &it.AdvisorName,
			&it.TaskNumber, &it.TaskName, &it.Reason, &it.DueDate, &it.Hours, &it.Value); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
