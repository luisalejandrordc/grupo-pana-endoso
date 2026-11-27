package dashboard

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// querier is the part of the connection pool the dashboard reads through.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// saleCodeSQL is the human-friendly sale number (EF-2026-00012) on the sale table known as s.
const saleCodeSQL = `'EF-' || to_char(s.created_at AT TIME ZONE ` + limaSQL + `, 'YYYY') || '-' || lpad(s.sale_number::text, 5, '0')`

// epochIn is the length of an interval in the given unit (3600.0 for hours, 86400.0 for days), as the double
// precision that percentile_cont needs.
func epochIn(interval, divisor string) string {
	return "(extract(epoch FROM (" + interval + ")) / " + divisor + ")::double precision"
}

// --- summary ---

type saleCounts struct{ active, created, completed, cancelled int }

func loadSaleCounts(ctx context.Context, q querier, w window, f Filter) (saleCounts, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	var c saleCounts
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE s.overall_state = 'active'),
			count(*) FILTER (WHERE s.created_at >= `+from+` AND s.created_at < `+to+`),
			count(*) FILTER (WHERE s.completed_at >= `+from+` AND s.completed_at < `+to+`),
			count(*) FILTER (WHERE s.cancelled_at >= `+from+` AND s.cancelled_at < `+to+`)
		FROM sale_registers s WHERE `+where, b.args...).Scan(&c.active, &c.created, &c.completed, &c.cancelled)
	return c, err
}

// loadTrend counts the created, completed and cancelled sales of every month of the window, zeros included.
func loadTrend(ctx context.Context, q querier, w window, f Filter) ([]TrendPoint, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `WITH ev AS (
			SELECT 'created' AS kind, s.created_at AS happened FROM sale_registers s
				WHERE `+where+` AND s.created_at >= `+from+` AND s.created_at < `+to+`
			UNION ALL
			SELECT 'completed', s.completed_at FROM sale_registers s
				WHERE `+where+` AND s.completed_at >= `+from+` AND s.completed_at < `+to+`
			UNION ALL
			SELECT 'cancelled', s.cancelled_at FROM sale_registers s
				WHERE `+where+` AND s.cancelled_at >= `+from+` AND s.cancelled_at < `+to+`
		)
		SELECT to_char(happened AT TIME ZONE `+limaSQL+`, 'YYYY-MM'), kind, count(*) FROM ev GROUP BY 1, 2`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := map[string]*TrendPoint{}
	months := w.months()
	out := make([]TrendPoint, len(months))
	for i, m := range months {
		out[i].Month = m
		points[m] = &out[i]
	}
	for rows.Next() {
		var month, kind string
		var n int
		if err := rows.Scan(&month, &kind, &n); err != nil {
			return nil, err
		}
		p := points[month]
		if p == nil {
			continue
		}
		switch kind {
		case "created":
			p.Created = n
		case "completed":
			p.Completed = n
		case "cancelled":
			p.Cancelled = n
		}
	}
	return out, rows.Err()
}

// loadMilestoneLoad counts the active sales in each of the six milestones, by the same derived state the
// customer portal uses.
func loadMilestoneLoad(ctx context.Context, q querier, f Filter) ([]MilestoneLoad, error) {
	var b builder
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `SELECT m.milestone_number, m.name_es, count(vm.sale_id)
		FROM milestone_definitions m
		LEFT JOIN v_sale_milestones vm ON vm.milestone_number = m.milestone_number AND vm.state = 'in_progress'
			AND vm.sale_id IN (SELECT s.id FROM sale_registers s WHERE s.overall_state = 'active' AND `+where+`)
		GROUP BY m.milestone_number, m.name_es ORDER BY m.milestone_number`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MilestoneLoad{}
	for rows.Next() {
		var m MilestoneLoad
		if err := rows.Scan(&m.Number, &m.Name, &m.ActiveSales); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// loadAttention finds every active sale that needs someone: its task in progress is late or about to be, or
// its next task has been able to start for longer than stalledAfter and nobody started it. A sale has at most
// one such task, because only one task of a sale can be in progress and a pending task is eligible only when
// the one before it is completed.
func loadAttention(ctx context.Context, q querier, f Filter, now time.Time) ([]AttentionItem, error) {
	var b builder
	nowArg := b.ts(now)
	stalledBefore := b.ts(now.Add(-stalledAfter))
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `SELECT s.id, `+saleCodeSQL+`, s.customer_legal_name_snapshot,
			s.vehicle_model_name_snapshot || ' ' || s.vehicle_version_name_snapshot,
			a.id, a.first_name || ' ' || a.last_name,
			t.task_number, d.name_es, t.status::text, t.due_date, e.eligible_at, v.approaching_at
		FROM sale_registers s
		JOIN employees a ON a.id = s.assigned_advisor_id
		JOIN task_instances t ON t.sale_id = s.id
		JOIN task_definitions d ON d.task_number = t.task_number
		LEFT JOIN v_task_eligibility e ON e.task_instance_id = t.id
		LEFT JOIN v_task_sla_state v ON v.task_instance_id = t.id
		WHERE s.overall_state = 'active' AND `+where+`
			AND (
				(t.status = 'in_progress' AND t.due_date IS NOT NULL AND (t.due_date < `+nowArg+` OR v.approaching_at <= `+nowArg+`))
				OR (t.status = 'pending' AND e.eligible_at IS NOT NULL AND e.eligible_at < `+stalledBefore+`)
			)`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttentionItem
	for rows.Next() {
		var it AttentionItem
		var status string
		var due, eligible, approaching *time.Time
		if err := rows.Scan(&it.SaleID, &it.SaleCode, &it.CustomerName, &it.Vehicle, &it.AdvisorID, &it.AdvisorName,
			&it.TaskNumber, &it.TaskName, &status, &due, &eligible, &approaching); err != nil {
			return nil, err
		}
		switch {
		case status == "in_progress" && due != nil && due.Before(now):
			it.Kind, it.DueDate = KindOverdue, due
		case status == "in_progress":
			it.Kind, it.DueDate = KindApproaching, due
		default:
			it.Kind, it.WaitingSince = KindStalled, eligible
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortAttention(out)
	return out, nil
}

// sortAttention puts overdue sales first (the longest overdue on top), then the ones about to be late (the
// soonest due first), then the stalled ones (waiting the longest first).
func sortAttention(items []AttentionItem) {
	rank := map[string]int{KindOverdue: 0, KindApproaching: 1, KindStalled: 2}
	key := func(it AttentionItem) time.Time {
		if it.Kind == KindStalled && it.WaitingSince != nil {
			return *it.WaitingSince
		}
		if it.DueDate != nil {
			return *it.DueDate
		}
		return time.Time{}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if rank[items[i].Kind] != rank[items[j].Kind] {
			return rank[items[i].Kind] < rank[items[j].Kind]
		}
		return key(items[i]).Before(key(items[j]))
	})
}

// --- process ---

func loadCycleTime(ctx context.Context, q querier, w window, f Filter) (CycleTime, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	days := epochIn("s.completed_at - s.created_at", "86400.0")
	var c CycleTime
	err := q.QueryRow(ctx, `SELECT count(*),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY `+days+`),
			percentile_cont(0.9) WITHIN GROUP (ORDER BY `+days+`)
		FROM sale_registers s
		WHERE s.completed_at >= `+from+` AND s.completed_at < `+to+` AND `+where, b.args...).Scan(&c.SampleSize, &c.MedianDays, &c.P90Days)
	c.MedianDays, c.P90Days = round1(c.MedianDays), round1(c.P90Days)
	return c, err
}

// loadReopenedTasks counts the completed tasks that were reopened in the period, the plainest sign of rework.
func loadReopenedTasks(ctx context.Context, q querier, w window, f Filter) (int, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM audit_logs l JOIN sale_registers s ON s.id = l.sale_id
		WHERE l.entity_type = 'task_instance' AND l.action = 'REOPENED'
			AND l.created_at >= `+from+` AND l.created_at < `+to+` AND `+where, b.args...).Scan(&n)
	return n, err
}

// loadTaskStats returns one row per task. ActiveSales counts the active sales whose current task it is;
// the other numbers come from the tasks completed in the period.
func loadTaskStats(ctx context.Context, q querier, w window, f Filter) ([]TaskStats, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	work := epochIn("t.completed_at - t.started_at", "3600.0")
	wait := epochIn("t.started_at - e.eligible_at", "3600.0")
	rows, err := q.Query(ctx, `SELECT d.task_number, d.name_es, d.milestone_number, d.recommended_duration_hours,
			coalesce(act.n, 0), coalesce(st.completed, 0), st.median_work, st.median_wait,
			coalesce(st.on_time, 0), coalesce(st.late, 0)
		FROM task_definitions d
		LEFT JOIN (
			SELECT c.current_task_number AS task_number, count(*) AS n
			FROM sale_registers s JOIN v_sale_current_task c ON c.sale_id = s.id
			WHERE s.overall_state = 'active' AND `+where+`
			GROUP BY 1
		) act ON act.task_number = d.task_number
		LEFT JOIN (
			SELECT t.task_number, count(*) AS completed,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY `+work+`) AS median_work,
				percentile_cont(0.5) WITHIN GROUP (ORDER BY `+wait+`)
					FILTER (WHERE e.eligible_at IS NOT NULL AND t.started_at >= e.eligible_at) AS median_wait,
				count(*) FILTER (WHERE t.completion_phase IN ('early', 'in_approaching')) AS on_time,
				count(*) FILTER (WHERE t.completion_phase IN ('overdue', 'recurring_overdue')) AS late
			FROM task_instances t
			JOIN sale_registers s ON s.id = t.sale_id
			LEFT JOIN v_task_eligibility e ON e.task_instance_id = t.id
			WHERE t.status = 'completed' AND t.completed_at >= `+from+` AND t.completed_at < `+to+` AND `+where+`
			GROUP BY t.task_number
		) st ON st.task_number = d.task_number
		ORDER BY d.task_number`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskStats{}
	for rows.Next() {
		var t TaskStats
		if err := rows.Scan(&t.Number, &t.Name, &t.Milestone, &t.RecommendedHours, &t.ActiveSales, &t.CompletedInPeriod,
			&t.MedianWorkHours, &t.MedianWaitHours, &t.OnTime, &t.Late); err != nil {
			return nil, err
		}
		t.MedianWorkHours, t.MedianWaitHours = round1(t.MedianWorkHours), round1(t.MedianWaitHours)
		if t.RecommendedHours != nil {
			t.OnTimeRate = rate(t.OnTime, t.Late)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadCancellations(ctx context.Context, q querier, w window, f Filter) (Cancellations, error) {
	out := Cancellations{ByCategory: []CategoryCount{}, ByTask: []TaskCount{}}

	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `SELECT s.cancellation_category::text, count(*) FROM sale_registers s
		WHERE s.cancelled_at >= `+from+` AND s.cancelled_at < `+to+` AND `+where+`
		GROUP BY 1 ORDER BY 2 DESC, 1`, b.args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c CategoryCount
		if err := rows.Scan(&c.Category, &c.Count); err != nil {
			rows.Close()
			return out, err
		}
		out.Total += c.Count
		out.ByCategory = append(out.ByCategory, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}

	var b2 builder
	from, to = b2.ts(w.start), b2.ts(w.end)
	where = f.where(&b2, "s")
	rows, err = q.Query(ctx, `SELECT s.cancelled_at_task_number, d.name_es, count(*) FROM sale_registers s
		JOIN task_definitions d ON d.task_number = s.cancelled_at_task_number
		WHERE s.cancelled_at >= `+from+` AND s.cancelled_at < `+to+` AND `+where+`
		GROUP BY 1, 2 ORDER BY 1`, b2.args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c TaskCount
		if err := rows.Scan(&c.Number, &c.Name, &c.Count); err != nil {
			return out, err
		}
		out.ByTask = append(out.ByTask, c)
	}
	return out, rows.Err()
}

// --- market ---

// mixGroup says how to group the sales created in the period. The three fragments are fixed text from this
// package, never input.
type mixGroup struct {
	key, label, groupBy string
	limit               int
}

var (
	groupModels        = mixGroup{key: "s.vehicle_model_id::text", label: "max(s.vehicle_model_name_snapshot)", groupBy: "s.vehicle_model_id", limit: 8}
	groupVersions      = mixGroup{key: "s.vehicle_version_id::text", label: "max(s.vehicle_model_name_snapshot || ' ' || s.vehicle_version_name_snapshot)", groupBy: "s.vehicle_version_id", limit: 8}
	groupColors        = mixGroup{key: "lower(s.vehicle_color_name_snapshot)", label: "min(s.vehicle_color_name_snapshot)", groupBy: "lower(s.vehicle_color_name_snapshot)", limit: 8}
	groupCustomerTypes = mixGroup{key: "s.customer_type_snapshot::text", label: "s.customer_type_snapshot::text", groupBy: "s.customer_type_snapshot"}
	groupTemperatures  = mixGroup{key: "s.sale_temperature::text", label: "s.sale_temperature::text", groupBy: "s.sale_temperature"}
)

func loadMix(ctx context.Context, q querier, w window, f Filter, g mixGroup) ([]MixItem, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	sql := `SELECT ` + g.key + `, ` + g.label + `, count(*),
			count(*) FILTER (WHERE s.overall_state = 'active'),
			count(*) FILTER (WHERE s.overall_state = 'completed'),
			count(*) FILTER (WHERE s.overall_state = 'cancelled')
		FROM sale_registers s
		WHERE s.created_at >= ` + from + ` AND s.created_at < ` + to + ` AND ` + where + `
		GROUP BY ` + g.groupBy + ` ORDER BY 3 DESC, 2`
	if g.limit > 0 {
		sql += ` LIMIT ` + b.add(g.limit)
	}
	rows, err := q.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MixItem{}
	for rows.Next() {
		var m MixItem
		if err := rows.Scan(&m.Key, &m.Label, &m.Total, &m.Active, &m.Completed, &m.Cancelled); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- finance ---

func loadPipelineAndCompleted(ctx context.Context, q querier, w window, f Filter) (PipelineMoney, CompletedMoney, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	active := "s.overall_state = 'active'"
	done := "s.overall_state = 'completed' AND s.completed_at >= " + from + " AND s.completed_at < " + to
	var p PipelineMoney
	var c CompletedMoney
	// Only sales whose quotation (task 1) is completed carry confirmed amounts. The commission of an active
	// sale is final when task 3 is completed too, and provisional before that.
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE `+active+`),
			`+money("f.quoted_final_price", active)+`,
			`+money("f.final_commission", active+" AND t3.id IS NOT NULL")+`,
			`+money("f.initial_commission", active+" AND t3.id IS NULL")+`,
			count(*) FILTER (WHERE `+done+`),
			`+money("f.quoted_final_price", done)+`,
			`+money("f.final_commission", done)+`,
			`+asMoney("coalesce(avg(f.final_commission) FILTER (WHERE "+done+"), 0)")+`,
			`+money("f.accessory_amount", done)+`,
			`+money("f.release_processing_amount", done)+`,
			`+money("f.final_toyota_amount", done)+`
		FROM sale_registers s
		JOIN sale_financials f ON f.sale_id = s.id
		JOIN task_instances t1 ON t1.sale_id = s.id AND t1.task_number = 1 AND t1.status = 'completed'
		LEFT JOIN task_instances t3 ON t3.sale_id = s.id AND t3.task_number = 3 AND t3.status = 'completed'
		WHERE `+where, b.args...).Scan(
		&p.Sales, &p.QuotedValue, &p.FinalCommission, &p.ProvisionalCommission,
		&c.Sales, &c.QuotedValue, &c.Commission, &c.AverageCommission, &c.Accessories, &c.ReleaseProcessing, &c.ToyotaAmount)
	return p, c, err
}

// loadCash reads the payments recorded in task 4 (customer to Grupo Pana, "pana") and task 5 (Grupo Pana to
// Toyota, "toyota"). A payment counts only once its task is completed.
func loadCash(ctx context.Context, q querier, w window, f Filter) (CashFlow, error) {
	var b builder
	fromDate, toDate := b.date(w.from), b.date(w.to)
	where := f.where(&b, "s")
	inPeriod := "p.payment_date >= " + fromDate + " AND p.payment_date <= " + toDate
	in, out := "p.payment_kind = 'pana'", "p.payment_kind = 'toyota'"
	active, cancelled := "s.overall_state = 'active'", "s.overall_state = 'cancelled'"
	var c CashFlow
	err := q.QueryRow(ctx, `SELECT
			`+money("p.deposited_amount", in+" AND "+inPeriod)+`,
			`+money("p.deposited_amount", out+" AND "+inPeriod)+`,
			`+asMoney(sumRaw("p.deposited_amount", in+" AND "+inPeriod)+" - "+sumRaw("p.deposited_amount", out+" AND "+inPeriod))+`,
			count(*) FILTER (WHERE `+inPeriod+` AND p.deposited_amount <> p.expected_amount),
			`+asMoney(sumRaw("p.deposited_amount", in+" AND "+active)+" - "+sumRaw("p.deposited_amount", out+" AND "+active))+`,
			`+money("p.deposited_amount", in+" AND "+cancelled)+`
		FROM sale_payments p
		JOIN sale_registers s ON s.id = p.sale_id
		JOIN task_instances t ON t.sale_id = p.sale_id AND t.status = 'completed'
			AND t.task_number = CASE p.payment_kind WHEN 'pana' THEN 4 ELSE 5 END
		WHERE `+where, b.args...).Scan(&c.MoneyIn, &c.MoneyOut, &c.Net, &c.PaymentMismatches, &c.HeldInActiveSales, &c.InCancelledSales)
	return c, err
}

// invoiceKinds is the order the four official invoices are shown in.
var invoiceKinds = []string{"toyota", "commission", "accessories", "release_processing"}

func loadInvoices(ctx context.Context, q querier, w window, f Filter) ([]InvoiceTotal, error) {
	var b builder
	fromDate, toDate := b.date(w.from), b.date(w.to)
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `SELECT i.invoice_kind::text, count(*), `+money("i.invoice_amount", "TRUE")+`,
			count(*) FILTER (WHERE i.invoice_amount <> i.expected_amount)
		FROM sale_invoices i
		JOIN sale_registers s ON s.id = i.sale_id
		JOIN task_instances t ON t.sale_id = i.sale_id AND t.task_number = 6 AND t.status = 'completed'
		WHERE i.invoice_date >= `+fromDate+` AND i.invoice_date <= `+toDate+` AND `+where+`
		GROUP BY 1`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]InvoiceTotal{}
	for rows.Next() {
		var it InvoiceTotal
		if err := rows.Scan(&it.Kind, &it.Count, &it.Amount, &it.Mismatches); err != nil {
			return nil, err
		}
		found[it.Kind] = it
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]InvoiceTotal, 0, len(invoiceKinds))
	for _, kind := range invoiceKinds {
		it, ok := found[kind]
		if !ok {
			it = InvoiceTotal{Kind: kind, Amount: "0.00"}
		}
		out = append(out, it)
	}
	return out, nil
}

// loadMoneyMonths gives, for every month of the window, the commission of the sales completed in it and the
// money received and paid out (by payment date).
func loadMoneyMonths(ctx context.Context, q querier, w window, f Filter) ([]MoneyMonth, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	fromDate, toDate := b.date(w.from), b.date(w.to)
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `WITH ev AS (
			SELECT to_char(s.completed_at AT TIME ZONE `+limaSQL+`, 'YYYY-MM') AS ym, 'commission' AS kind, f.final_commission AS amount
			FROM sale_registers s JOIN sale_financials f ON f.sale_id = s.id
			WHERE s.overall_state = 'completed' AND s.completed_at >= `+from+` AND s.completed_at < `+to+` AND `+where+`
			UNION ALL
			SELECT to_char(p.payment_date, 'YYYY-MM'), CASE p.payment_kind WHEN 'pana' THEN 'in' ELSE 'out' END, p.deposited_amount
			FROM sale_payments p
			JOIN sale_registers s ON s.id = p.sale_id
			JOIN task_instances t ON t.sale_id = p.sale_id AND t.status = 'completed'
				AND t.task_number = CASE p.payment_kind WHEN 'pana' THEN 4 ELSE 5 END
			WHERE p.payment_date >= `+fromDate+` AND p.payment_date <= `+toDate+` AND `+where+`
		)
		SELECT ym, kind, (sum(amount))::numeric(14,2)::text FROM ev GROUP BY 1, 2`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	months := w.months()
	out := make([]MoneyMonth, len(months))
	at := map[string]*MoneyMonth{}
	for i, m := range months {
		out[i] = MoneyMonth{Month: m, Commission: "0.00", MoneyIn: "0.00", MoneyOut: "0.00"}
		at[m] = &out[i]
	}
	for rows.Next() {
		var month, kind, amount string
		if err := rows.Scan(&month, &kind, &amount); err != nil {
			return nil, err
		}
		m := at[month]
		if m == nil {
			continue
		}
		switch kind {
		case "commission":
			m.Commission = amount
		case "in":
			m.MoneyIn = amount
		case "out":
			m.MoneyOut = amount
		}
	}
	return out, rows.Err()
}

// --- advisors ---

func loadAdvisorRows(ctx context.Context, q querier, w window, f Filter) ([]AdvisorRow, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	where := f.where(&b, "s")
	completed := "s.completed_at >= " + from + " AND s.completed_at < " + to
	rows, err := q.Query(ctx, `SELECT a.id, a.first_name || ' ' || a.last_name, a.is_active, a.profile_picture_key IS NOT NULL,
			count(*) FILTER (WHERE s.overall_state = 'active'),
			count(*) FILTER (WHERE s.created_at >= `+from+` AND s.created_at < `+to+`),
			count(*) FILTER (WHERE `+completed+`),
			count(*) FILTER (WHERE s.cancelled_at >= `+from+` AND s.cancelled_at < `+to+`),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY `+epochIn("s.completed_at - s.created_at", "86400.0")+`) FILTER (WHERE `+completed+`),
			`+money("f.quoted_final_price", "s.overall_state = 'active' AND t1.id IS NOT NULL")+`,
			`+money("f.final_commission", completed)+`
		FROM sale_registers s
		JOIN employees a ON a.id = s.assigned_advisor_id
		LEFT JOIN sale_financials f ON f.sale_id = s.id
		LEFT JOIN task_instances t1 ON t1.sale_id = s.id AND t1.task_number = 1 AND t1.status = 'completed'
		WHERE `+where+`
		GROUP BY a.id, a.first_name, a.last_name, a.is_active, a.profile_picture_key`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdvisorRow{}
	for rows.Next() {
		var r AdvisorRow
		if err := rows.Scan(&r.AdvisorID, &r.Name, &r.IsActive, &r.HasPhoto, &r.ActiveSales, &r.CreatedInPeriod,
			&r.CompletedInPeriod, &r.CancelledInPeriod, &r.MedianCycleDays, &r.PipelineValue, &r.Commission); err != nil {
			return nil, err
		}
		r.MedianCycleDays = round1(r.MedianCycleDays)
		r.ClosingRate = rate(r.CompletedInPeriod, r.CancelledInPeriod)
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadOverdueByAdvisor counts, per advisor, the tasks in progress that are past their due date today.
func loadOverdueByAdvisor(ctx context.Context, q querier, f Filter, now time.Time) (map[uuid.UUID]int, error) {
	var b builder
	nowArg := b.ts(now)
	where := f.where(&b, "s")
	rows, err := q.Query(ctx, `SELECT s.assigned_advisor_id, count(*) FROM task_instances t
		JOIN sale_registers s ON s.id = t.sale_id
		WHERE t.status = 'in_progress' AND t.due_date < `+nowArg+` AND s.overall_state = 'active' AND `+where+`
		GROUP BY 1`, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]int{}
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// --- system ---

func loadStaffStats(ctx context.Context, q querier, now time.Time) (StaffStats, error) {
	var b builder
	nowArg := b.ts(now)
	var s StaffStats
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE role = 'system_admin' AND is_active),
			count(*) FILTER (WHERE role = 'sales_manager' AND is_active),
			count(*) FILTER (WHERE role = 'sales_advisor' AND is_active),
			count(*) FILTER (WHERE NOT is_active),
			count(*) FILTER (WHERE locked_until > `+nowArg+`)
		FROM employees WHERE NOT is_system`, b.args...).Scan(&s.Admins, &s.Managers, &s.Advisors, &s.Inactive, &s.LockedNow)
	return s, err
}

func loadNotificationStats(ctx context.Context, q querier, w window) (NotificationStats, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	inPeriod := "created_at >= " + from + " AND created_at < " + to
	var n NotificationStats
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE status = 'pending'),
			min(created_at) FILTER (WHERE status = 'pending'),
			count(*) FILTER (WHERE status = 'sent' AND `+inPeriod+`),
			count(*) FILTER (WHERE status = 'failed' AND `+inPeriod+`),
			count(*) FILTER (WHERE status = 'skipped' AND `+inPeriod+`)
		FROM notification_logs`, b.args...).Scan(&n.PendingNow, &n.OldestPendingAt, &n.Sent, &n.Failed, &n.Skipped)
	return n, err
}

func loadSecurityStats(ctx context.Context, q querier, w window) (SecurityStats, error) {
	var b builder
	from, to := b.ts(w.start), b.ts(w.end)
	var s SecurityStats
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE action_type = 'login_success'),
			count(*) FILTER (WHERE action_type = 'login_failed'),
			count(*) FILTER (WHERE action_type = 'login_failed' AND failure_reason = 'account_locked'),
			count(*) FILTER (WHERE action_type = 'password_reset_completed')
		FROM access_logs
		WHERE account_type = 'employee' AND created_at >= `+from+` AND created_at < `+to, b.args...).
		Scan(&s.SuccessfulLogins, &s.FailedLogins, &s.LockedAttempts, &s.PasswordResets)
	return s, err
}
