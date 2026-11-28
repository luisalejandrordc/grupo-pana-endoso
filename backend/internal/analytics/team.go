package analytics

import (
	"context"
	"endosoflow/internal/auth"
)

// Team answers the team tab: one row per advisor. Only Admins and Managers may ask, and the advisor
// filter does not apply (every advisor is a row).
func (s *Service) Team(ctx context.Context, actor auth.Actor, f Filter) (*Team, error) {
	if !seesInternalMoney(actor) {
		return nil, errForbidden
	}
	sc, err := s.scopeFor(actor, f, true)
	if err != nil {
		return nil, err
	}
	sc.advisor = nil
	out := &Team{Meta: s.meta(sc), Advisors: []AdvisorPerformance{}}
	err = s.read(ctx, func(q querier) error {
		cycle := `extract(epoch FROM fs.completed_at - fs.created_at) / 3600.0`
		completed := inPeriod("fs.completed_at")
		rows, err := q.Query(ctx, cteP+cteFS+cteCur+cteCl+`, agg AS (
	SELECT fs.assigned_advisor_id AS advisor_id,
		count(*) FILTER (WHERE fs.overall_state = 'active')::int AS active,
		count(*) FILTER (WHERE `+inPeriod("fs.created_at")+`)::int AS created,
		count(*) FILTER (WHERE `+completed+`)::int AS completed,
		count(*) FILTER (WHERE `+inPeriod("fs.cancelled_at")+`)::int AS cancelled,
		`+money("sum(fs.value) FILTER (WHERE fs.overall_state = 'active')")+` AS pipeline,
		`+money("sum(fs.value) FILTER (WHERE "+completed+")")+` AS completed_value,
		`+money("sum(fs.final_commission) FILTER (WHERE "+completed+")")+` AS commission,
		percentile_cont(0.5) WITHIN GROUP (ORDER BY `+cycle+`) FILTER (WHERE `+completed+`) AS cycle
	FROM fs CROSS JOIN p GROUP BY fs.assigned_advisor_id),
ld AS (
	SELECT advisor_id, count(*) FILTER (WHERE kind = 'overdue')::int AS overdue, count(*) FILTER (WHERE kind = 'idle')::int AS idle
	FROM cl GROUP BY advisor_id),
sla AS (
	SELECT fs.assigned_advisor_id AS advisor_id,
		count(*) FILTER (WHERE t.completion_phase IN ('early', 'in_approaching'))::int AS num, count(*)::int AS den
	FROM task_instances t JOIN fs ON fs.id = t.sale_id CROSS JOIN p
	WHERE t.status = 'completed' AND t.completion_phase <> 'not_applicable' AND `+inPeriod("t.completed_at")+`
	GROUP BY fs.assigned_advisor_id)
SELECT e.id::text, e.first_name || ' ' || e.last_name, e.is_active,
	agg.active, coalesce(ld.overdue, 0), coalesce(ld.idle, 0),
	agg.created, agg.completed, agg.cancelled, agg.cycle,
	coalesce(sla.num, 0), coalesce(sla.den, 0), agg.pipeline, agg.completed_value, agg.commission
FROM agg
JOIN employees e ON e.id = agg.advisor_id
LEFT JOIN ld ON ld.advisor_id = agg.advisor_id
LEFT JOIN sla ON sla.advisor_id = agg.advisor_id
WHERE agg.active > 0 OR agg.created > 0 OR agg.completed > 0 OR agg.cancelled > 0
ORDER BY lower(e.first_name || ' ' || e.last_name), e.id`, sc.args()...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a AdvisorPerformance
			var slaNum, slaDen int
			if err := rows.Scan(&a.AdvisorID, &a.Name, &a.IsActive, &a.Active, &a.Overdue, &a.Idle,
				&a.Created, &a.Completed, &a.Cancelled, &a.CycleMedianHours,
				&slaNum, &slaDen, &a.PipelineValue, &a.CompletedValue, &a.Commission); err != nil {
				return err
			}
			a.CloseRate = mkRatio(a.Completed, a.Completed+a.Cancelled)
			a.OnTimeRate = mkRatio(slaNum, slaDen)
			out.Advisors = append(out.Advisors, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
