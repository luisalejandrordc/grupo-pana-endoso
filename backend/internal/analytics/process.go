package analytics

import (
	"context"
	"endosoflow/internal/auth"
)

// Process answers the process tab: where time goes, where deadlines slip and where work is redone.
func (s *Service) Process(ctx context.Context, actor auth.Actor, f Filter) (*Process, error) {
	sc, err := s.scopeFor(actor, f, true)
	if err != nil {
		return nil, err
	}
	out := &Process{Meta: s.meta(sc)}
	err = s.read(ctx, func(q querier) error {
		tasks, err := loadTaskDefinitions(ctx, q, sc)
		if err != nil {
			return err
		}
		byNumber := map[int]*TaskTiming{}
		for i := range tasks {
			byNumber[tasks[i].TaskNumber] = &tasks[i]
		}
		if err := loadTaskTimings(ctx, q, sc, byNumber); err != nil {
			return err
		}
		if err := loadTaskLoad(ctx, q, sc, byNumber); err != nil {
			return err
		}
		if err := loadRework(ctx, q, sc, byNumber); err != nil {
			return err
		}
		out.Tasks = tasks
		if out.OnTimeRate, err = loadOnTimeRate(ctx, q, sc); err != nil {
			return err
		}
		out.Customs, err = loadCustoms(ctx, q, sc)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func loadTaskDefinitions(ctx context.Context, q querier, sc scope) ([]TaskTiming, error) {
	rows, err := q.Query(ctx, `SELECT task_number, name_es, milestone_number, recommended_duration_hours
FROM task_definitions ORDER BY task_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskTiming{}
	for rows.Next() {
		var t TaskTiming
		if err := rows.Scan(&t.TaskNumber, &t.Name, &t.MilestoneNumber, &t.RecommendedHours); err != nil {
			return nil, err
		}
		if t.RecommendedHours != nil {
			t.Deadlines = &Deadlines{}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadTaskTimings reads the tasks completed in the period: work time, waiting time and deadline phase.
// Waiting is only measured when the task started after it became eligible; a reopened earlier task can
// move the eligibility past the start, and that negative wait is not a real wait.
func loadTaskTimings(ctx context.Context, q querier, sc scope, byNumber map[int]*TaskTiming) error {
	work := `extract(epoch FROM t.completed_at - t.started_at) / 3600.0`
	wait := `extract(epoch FROM t.started_at - el.eligible_at) / 3600.0`
	rows, err := q.Query(ctx, cteP+cteFS+`
SELECT t.task_number, count(*)::int,
	percentile_cont(0.5) WITHIN GROUP (ORDER BY `+work+`),
	percentile_cont(0.9) WITHIN GROUP (ORDER BY `+work+`),
	percentile_cont(0.5) WITHIN GROUP (ORDER BY `+wait+`) FILTER (WHERE el.eligible_at IS NOT NULL AND t.started_at >= el.eligible_at),
	count(*) FILTER (WHERE t.completion_phase = 'early')::int,
	count(*) FILTER (WHERE t.completion_phase = 'in_approaching')::int,
	count(*) FILTER (WHERE t.completion_phase = 'overdue')::int,
	count(*) FILTER (WHERE t.completion_phase = 'recurring_overdue')::int
FROM task_instances t
JOIN fs ON fs.id = t.sale_id
JOIN v_task_eligibility el ON el.task_instance_id = t.id
CROSS JOIN p
WHERE t.status = 'completed' AND t.started_at IS NOT NULL AND `+inPeriod("t.completed_at")+`
GROUP BY t.task_number`, sc.args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var n, count, early, approaching, overdue, recurring int
		var median, p90, waitMedian *float64
		if err := rows.Scan(&n, &count, &median, &p90, &waitMedian, &early, &approaching, &overdue, &recurring); err != nil {
			return err
		}
		t := byNumber[n]
		if t == nil {
			continue
		}
		t.CompletedCount, t.WorkMedianHours, t.WorkP90Hours, t.WaitMedianHours = count, median, p90, waitMedian
		if t.Deadlines != nil {
			*t.Deadlines = Deadlines{Early: early, InApproaching: approaching, Overdue: overdue, RecurringOverdue: recurring}
		}
	}
	return rows.Err()
}

// loadTaskLoad counts, per task, the active sales that are on it now and how many of those are overdue.
func loadTaskLoad(ctx context.Context, q querier, sc scope, byNumber map[int]*TaskTiming) error {
	rows, err := q.Query(ctx, cteP+cteFS+cteCur+cteCl+`
SELECT task_number, count(*)::int, count(*) FILTER (WHERE kind = 'overdue')::int FROM cl GROUP BY task_number`, sc.args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var n, current, overdue int
		if err := rows.Scan(&n, &current, &overdue); err != nil {
			return err
		}
		if t := byNumber[n]; t != nil {
			t.CurrentCount, t.OverdueCount = current, overdue
		}
	}
	return rows.Err()
}

// loadRework counts the audit events that redo a task: restarts, reopenings and corrections.
func loadRework(ctx context.Context, q querier, sc scope, byNumber map[int]*TaskTiming) error {
	rows, err := q.Query(ctx, cteP+cteFS+`
SELECT t.task_number, count(*)::int
FROM audit_logs a
JOIN task_instances t ON t.id = a.entity_id
JOIN fs ON fs.id = t.sale_id
CROSS JOIN p
WHERE a.entity_type = 'task_instance' AND a.action IN ('RESTARTED', 'REOPENED', 'CORRECTED')
  AND `+inPeriod("a.created_at")+`
GROUP BY t.task_number`, sc.args()...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var n, count int
		if err := rows.Scan(&n, &count); err != nil {
			return err
		}
		if t := byNumber[n]; t != nil {
			t.ReworkCount = count
		}
	}
	return rows.Err()
}

// loadOnTimeRate is the share of tasks with a deadline that were completed early or while approaching it.
func loadOnTimeRate(ctx context.Context, q querier, sc scope) (RatioMetric, error) {
	var out RatioMetric
	var prevNum, prevDen int
	good := "t.completion_phase IN ('early', 'in_approaching')"
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT
	count(*) FILTER (WHERE `+inPeriod("t.completed_at")+` AND `+good+`)::int,
	count(*) FILTER (WHERE `+inPeriod("t.completed_at")+`)::int,
	count(*) FILTER (WHERE `+inPrevious("t.completed_at")+` AND `+good+`)::int,
	count(*) FILTER (WHERE `+inPrevious("t.completed_at")+`)::int
FROM task_instances t
JOIN fs ON fs.id = t.sale_id
CROSS JOIN p
WHERE t.status = 'completed' AND t.completion_phase <> 'not_applicable'
  AND t.completed_at >= p.prev_from_ts AND t.completed_at < p.to_ts`, sc.args()...).Scan(&out.Numerator, &out.Denominator, &prevNum, &prevDen)
	if err != nil {
		return out, err
	}
	out.Value, out.Previous = ratio(out.Numerator, out.Denominator), ratio(prevNum, prevDen)
	return out, nil
}

// loadCustoms is the customs task (8) by control channel.
func loadCustoms(ctx context.Context, q querier, sc scope) ([]CustomsGroup, error) {
	rows, err := q.Query(ctx, cteP+cteFS+`
SELECT sf.control_channel::text, count(*)::int,
	percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.completed_at - t.started_at) / 3600.0)
FROM task_instances t
JOIN fs ON fs.id = t.sale_id
JOIN sale_fulfillment sf ON sf.sale_id = t.sale_id
CROSS JOIN p
WHERE t.task_number = 8 AND t.status = 'completed' AND t.started_at IS NOT NULL
  AND sf.control_channel IS NOT NULL AND `+inPeriod("t.completed_at")+`
GROUP BY sf.control_channel
ORDER BY sf.control_channel`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomsGroup{}
	for rows.Next() {
		var g CustomsGroup
		if err := rows.Scan(&g.Channel, &g.Count, &g.WorkMedianHours); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
