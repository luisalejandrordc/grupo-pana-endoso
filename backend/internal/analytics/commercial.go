package analytics

import (
	"context"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"time"
)

// cteCo is the cohort of the commercial tab: the sales created in the period.
const cteCo = `, co AS (SELECT fs.* FROM fs CROSS JOIN p WHERE fs.created_at >= p.from_ts AND fs.created_at < p.to_ts)`

// Commercial answers the commercial tab: what sells, what is lost and the money behind it.
func (s *Service) Commercial(ctx context.Context, actor auth.Actor, f Filter) (*Commercial, error) {
	sc, err := s.scopeFor(actor, f, true)
	if err != nil {
		return nil, err
	}
	groupBy := f.GroupBy
	if groupBy == "" {
		groupBy = "model"
	}
	internal := seesInternalMoney(actor)
	out := &Commercial{Meta: s.meta(sc)}
	err = s.read(ctx, func(q querier) error {
		var err error
		if out.Vehicles, err = loadVehicleGroups(ctx, q, sc, groupBy, internal); err != nil {
			return err
		}
		if out.CustomerTypes, err = loadCustomerTypes(ctx, q, sc, internal); err != nil {
			return err
		}
		if out.Temperatures, err = loadTemperatures(ctx, q, sc); err != nil {
			return err
		}
		if out.Cancellations, err = loadCancellations(ctx, q, sc); err != nil {
			return err
		}
		if out.ValueTrend, err = loadValueTrend(ctx, q, sc, internal); err != nil {
			return err
		}
		if internal {
			if out.PriceComposition, err = loadPriceComposition(ctx, q, sc); err != nil {
				return err
			}
		}
		out.InvoicedByKind, err = loadInvoicedByKind(ctx, q, sc)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func loadVehicleGroups(ctx context.Context, q querier, sc scope, groupBy string, internal bool) ([]VehicleGroup, error) {
	var from, key, label string
	switch groupBy {
	case "model":
		from, key, label = "co", "co.vehicle_model_id::text", "co.vehicle_model_name_snapshot"
	case "version":
		from, key, label = "co", "co.vehicle_version_id::text", "co.vehicle_model_name_snapshot || ' ' || co.vehicle_version_name_snapshot"
	case "color":
		from, key, label = "co", "co.vehicle_color_id::text", "co.vehicle_model_name_snapshot || ' ' || co.vehicle_color_name_snapshot"
	case "category":
		from, key, label = "co JOIN vehicle_model_categories vmc ON vmc.model_id = co.vehicle_model_id", "vmc.category::text", "vmc.category::text"
	default:
		return nil, apperr.BadRequest("INVALID_QUERY", "El agrupamiento solicitado no es válido.")
	}
	rows, err := q.Query(ctx, cteP+cteFS+cteCo+`
SELECT g.gkey, g.glabel, count(*)::int,
	count(*) FILTER (WHERE g.overall_state = 'completed')::int,
	count(*) FILTER (WHERE g.overall_state = 'cancelled')::int,
	count(*) FILTER (WHERE g.overall_state = 'active')::int,
	`+money("sum(g.value) FILTER (WHERE g.overall_state <> 'cancelled')")+`,
	(avg(g.value) FILTER (WHERE g.overall_state = 'completed'))::numeric(14,2)::text,
	`+money("sum(g.final_commission) FILTER (WHERE g.overall_state = 'completed')")+`
FROM (SELECT co.overall_state, co.value, co.final_commission, `+key+` AS gkey, `+label+` AS glabel FROM `+from+`) g
GROUP BY g.gkey, g.glabel
ORDER BY count(*) DESC, g.glabel
LIMIT 10`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VehicleGroup{}
	for rows.Next() {
		var g VehicleGroup
		var commission string
		if err := rows.Scan(&g.Key, &g.Label, &g.Created, &g.Completed, &g.Cancelled, &g.Active, &g.QuotedValue, &g.AverageTicket, &commission); err != nil {
			return nil, err
		}
		if internal {
			g.Commission = &commission
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadCustomerTypes(ctx context.Context, q querier, sc scope, internal bool) ([]CustomerTypeGroup, error) {
	rows, err := q.Query(ctx, cteP+cteFS+cteCo+`
SELECT co.customer_type_snapshot::text, count(*)::int,
	count(*) FILTER (WHERE co.overall_state = 'completed')::int,
	count(*) FILTER (WHERE co.overall_state = 'cancelled')::int,
	count(*) FILTER (WHERE co.overall_state = 'active')::int,
	`+money("sum(co.value) FILTER (WHERE co.overall_state <> 'cancelled')")+`,
	`+money("sum(co.final_commission) FILTER (WHERE co.overall_state = 'completed')")+`
FROM co
GROUP BY co.customer_type_snapshot
ORDER BY count(*) DESC, co.customer_type_snapshot::text`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerTypeGroup{}
	for rows.Next() {
		var g CustomerTypeGroup
		var commission string
		if err := rows.Scan(&g.CustomerType, &g.Created, &g.Completed, &g.Cancelled, &g.Active, &g.QuotedValue, &commission); err != nil {
			return nil, err
		}
		g.CloseRate = mkRatio(g.Completed, g.Completed+g.Cancelled)
		if internal {
			g.Commission = &commission
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// loadTemperatures always returns hot, warm and cold, in that order, even when a group has no sales.
func loadTemperatures(ctx context.Context, q querier, sc scope) ([]TemperatureGroup, error) {
	rows, err := q.Query(ctx, cteP+cteFS+cteCo+`
SELECT co.sale_temperature::text, count(*)::int,
	count(*) FILTER (WHERE co.overall_state = 'completed')::int,
	count(*) FILTER (WHERE co.overall_state = 'cancelled')::int,
	count(*) FILTER (WHERE co.overall_state = 'active')::int
FROM co GROUP BY co.sale_temperature`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]TemperatureGroup{}
	for rows.Next() {
		var g TemperatureGroup
		if err := rows.Scan(&g.Temperature, &g.Created, &g.Completed, &g.Cancelled, &g.InProgress); err != nil {
			return nil, err
		}
		g.CloseRate = mkRatio(g.Completed, g.Completed+g.Cancelled)
		found[g.Temperature] = g
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]TemperatureGroup, 0, 3)
	for _, t := range []string{"hot", "warm", "cold"} {
		g, ok := found[t]
		if !ok {
			g = TemperatureGroup{Temperature: t, CloseRate: mkRatio(0, 0)}
		}
		out = append(out, g)
	}
	return out, nil
}

func loadCancellations(ctx context.Context, q querier, sc scope) (Cancellations, error) {
	out := Cancellations{ByCategory: []CancellationReason{}, ByTask: []CancellationTask{}}
	cancelled := "fs.overall_state = 'cancelled' AND " + inPeriod("fs.cancelled_at")
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT `+money("sum(fs.value)")+`, count(*)::int, count(*) FILTER (WHERE fs.quoted IS NULL)::int
FROM fs CROSS JOIN p WHERE `+cancelled, sc.args()...).Scan(&out.Total.Amount, &out.Total.SaleCount, &out.Total.EstimatedCount)
	if err != nil {
		return out, err
	}
	rows, err := q.Query(ctx, cteP+cteFS+`
SELECT fs.cancellation_category::text, count(*)::int, `+money("sum(fs.value)")+`
FROM fs CROSS JOIN p WHERE `+cancelled+`
GROUP BY fs.cancellation_category ORDER BY count(*) DESC, fs.cancellation_category::text`, sc.args()...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r CancellationReason
		if err := rows.Scan(&r.Category, &r.Count, &r.Value); err != nil {
			rows.Close()
			return out, err
		}
		out.ByCategory = append(out.ByCategory, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	rows, err = q.Query(ctx, cteP+cteFS+`
SELECT fs.cancelled_at_task_number::int, td.name_es, count(*)::int
FROM fs JOIN task_definitions td ON td.task_number = fs.cancelled_at_task_number CROSS JOIN p
WHERE `+cancelled+`
GROUP BY fs.cancelled_at_task_number, td.name_es ORDER BY fs.cancelled_at_task_number`, sc.args()...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var t CancellationTask
		if err := rows.Scan(&t.TaskNumber, &t.Name, &t.Count); err != nil {
			return out, err
		}
		out.ByTask = append(out.ByTask, t)
	}
	return out, rows.Err()
}

func loadValueTrend(ctx context.Context, q querier, sc scope, internal bool) ([]ValueBucket, error) {
	b, err := bucketCTE(sc.period.Granularity)
	if err != nil {
		return nil, err
	}
	g := sc.period.Granularity
	rows, err := q.Query(ctx, cteP+cteFS+b+`, ev AS (
	SELECT `+bucketOf(g, "fs.created_at")+` AS bucket, fs.value AS created_value, 0::numeric AS completed_value, 0::numeric AS commission
		FROM fs CROSS JOIN p WHERE `+inPeriod("fs.created_at")+`
	UNION ALL
	SELECT `+bucketOf(g, "fs.completed_at")+`, 0, fs.value, coalesce(fs.final_commission, 0)
		FROM fs CROSS JOIN p WHERE fs.overall_state = 'completed' AND `+inPeriod("fs.completed_at")+`)
SELECT b.bucket, `+money("sum(ev.created_value)")+`, `+money("sum(ev.completed_value)")+`, `+money("sum(ev.commission)")+`
FROM b LEFT JOIN ev ON ev.bucket = b.bucket
GROUP BY b.bucket ORDER BY b.bucket`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ValueBucket{}
	for rows.Next() {
		var bucket time.Time
		var v ValueBucket
		var commission string
		if err := rows.Scan(&bucket, &v.CreatedValue, &v.CompletedValue, &commission); err != nil {
			return nil, err
		}
		v.Bucket = bucket.Format(dateLayout)
		if internal {
			v.Commission = &commission
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// loadPriceComposition splits the quoted price of the sales completed in the period into its four parts.
func loadPriceComposition(ctx context.Context, q querier, sc scope) (*PriceComposition, error) {
	var c PriceComposition
	err := q.QueryRow(ctx, cteP+cteFS+`
SELECT `+money("sum(fs.final_toyota_amount)")+`, `+money("sum(fs.final_commission)")+`,
	`+money("sum(fs.accessories)")+`, `+money("sum(fs.release_processing)")+`, count(*)::int
FROM fs CROSS JOIN p
WHERE fs.overall_state = 'completed' AND `+inPeriod("fs.completed_at"), sc.args()...).Scan(
		&c.Toyota, &c.Commission, &c.Accessories, &c.ReleaseProcessing, &c.SaleCount)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func loadInvoicedByKind(ctx context.Context, q querier, sc scope) ([]InvoicedKind, error) {
	rows, err := q.Query(ctx, cteP+cteFS+`
SELECT si.invoice_kind::text, count(*)::int, `+money("sum(si.invoice_amount)")+`
FROM fs JOIN sale_invoices si ON si.sale_id = fs.id CROSS JOIN p
WHERE fs.overall_state <> 'cancelled' AND si.invoice_amount IS NOT NULL AND `+onDates("si.invoice_date")+`
GROUP BY si.invoice_kind ORDER BY si.invoice_kind::text`, sc.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InvoicedKind{}
	for rows.Next() {
		var k InvoicedKind
		if err := rows.Scan(&k.Kind, &k.Count, &k.Amount); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
