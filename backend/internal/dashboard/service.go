package dashboard

import (
	"context"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// attentionLimit is how many sales the "needs attention" list carries; the total counts all of them.
const attentionLimit = 10

// maxParallel is how many queries one request runs at the same time. The connection pool is small (the
// hosting tiers allow few connections), so a dashboard tab must never take all of it.
const maxParallel = 2

var errForbidden = apperr.Forbidden("FORBIDDEN", "No tiene permiso para realizar esta acción.")

// Database is what the service reads through: the connection pool.
type Database interface {
	querier
}

// Deps are the service's dependencies.
type Deps struct {
	DB     Database
	Logger *slog.Logger
	Now    func() time.Time // nil means time.Now
}

// Service answers the dashboard questions. Every method takes the actor and decides what that role may see.
type Service struct {
	db     querier
	logger *slog.Logger
	now    func() time.Time
}

// NewService builds the service.
func NewService(d Deps) *Service {
	s := &Service{db: d.DB, logger: d.Logger, now: d.Now}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func isStaff(a auth.Actor) bool {
	return a.Role == auth.RoleSystemAdmin || a.Role == auth.RoleSalesManager || a.Role == auth.RoleSalesAdvisor
}

// prepare applies the rules every tab shares: only staff read the dashboard, an Advisor only ever sees their
// own sales (whatever advisor the request names), and the period is resolved against today in Lima.
func (s *Service) prepare(actor auth.Actor, f Filter) (Filter, window, time.Time, error) {
	if !isStaff(actor) {
		return f, window{}, time.Time{}, errForbidden
	}
	if actor.Role == auth.RoleSalesAdvisor {
		id := actor.ID
		f.AdvisorID = &id
	}
	now := s.now()
	w, err := resolveWindow(f, now)
	return f, w, now, err
}

// run executes the functions with at most maxParallel at a time and returns the first error.
func run(fns ...func() error) error {
	errs := make([]error, len(fns))
	slots := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			errs[i] = fn()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Summary is the first tab.
func (s *Service) Summary(ctx context.Context, actor auth.Actor, f Filter) (*Summary, error) {
	f, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	var (
		counts    saleCounts
		trend     []TrendPoint
		load      []MilestoneLoad
		attention []AttentionItem
	)
	err = run(
		func() (err error) { counts, err = loadSaleCounts(ctx, s.db, w, f); return },
		func() (err error) { trend, err = loadTrend(ctx, s.db, w, f); return },
		func() (err error) { load, err = loadMilestoneLoad(ctx, s.db, f); return },
		func() (err error) { attention, err = loadAttention(ctx, s.db, f, now); return },
	)
	if err != nil {
		return nil, err
	}
	kpis := SummaryKpis{
		ActiveSales:       counts.active,
		CreatedInPeriod:   counts.created,
		CompletedInPeriod: counts.completed,
		CancelledInPeriod: counts.cancelled,
		ClosingRate:       rate(counts.completed, counts.cancelled),
	}
	for _, it := range attention {
		switch it.Kind {
		case KindOverdue:
			kpis.OverdueTasks++
		case KindApproaching:
			kpis.ApproachingTasks++
		case KindStalled:
			kpis.StalledTasks++
		}
	}
	list := AttentionList{Total: len(attention), Items: attention}
	if len(list.Items) > attentionLimit {
		list.Items = list.Items[:attentionLimit]
	}
	if list.Items == nil {
		list.Items = []AttentionItem{}
	}
	return &Summary{Period: w.period(), GeneratedAt: now, Kpis: kpis, Trend: trend, Milestones: load, Attention: list}, nil
}

// Process is the tab about how the work flows.
func (s *Service) Process(ctx context.Context, actor auth.Actor, f Filter) (*Process, error) {
	f, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	out := &Process{Period: w.period(), GeneratedAt: now}
	err = run(
		func() (err error) { out.CycleTime, err = loadCycleTime(ctx, s.db, w, f); return },
		func() (err error) { out.ReopenedTasks, err = loadReopenedTasks(ctx, s.db, w, f); return },
		func() (err error) { out.Tasks, err = loadTaskStats(ctx, s.db, w, f); return },
		func() (err error) { out.Cancellations, err = loadCancellations(ctx, s.db, w, f); return },
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// temperatureOrder is how the three temperatures are always listed.
var temperatureOrder = []string{"hot", "warm", "cold"}

// Market is the tab about what is sold and to whom.
func (s *Service) Market(ctx context.Context, actor auth.Actor, f Filter) (*Market, error) {
	f, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	out := &Market{Period: w.period(), GeneratedAt: now}
	var temps []MixItem
	err = run(
		func() (err error) { out.Models, err = loadMix(ctx, s.db, w, f, groupModels); return },
		func() (err error) { out.Versions, err = loadMix(ctx, s.db, w, f, groupVersions); return },
		func() (err error) { out.Colors, err = loadMix(ctx, s.db, w, f, groupColors); return },
		func() (err error) { out.CustomerTypes, err = loadMix(ctx, s.db, w, f, groupCustomerTypes); return },
		func() (err error) { temps, err = loadMix(ctx, s.db, w, f, groupTemperatures); return },
	)
	if err != nil {
		return nil, err
	}
	byTemp := map[string]MixItem{}
	for _, t := range temps {
		byTemp[t.Key] = t
	}
	out.Temperatures = make([]TemperatureStats, 0, len(temperatureOrder))
	for _, key := range temperatureOrder {
		t := byTemp[key]
		out.Temperatures = append(out.Temperatures, TemperatureStats{
			Temperature: key, Total: t.Total, Active: t.Active, Completed: t.Completed, Cancelled: t.Cancelled,
			ClosingRate: rate(t.Completed, t.Cancelled),
		})
	}
	return out, nil
}

// Finance is the tab about recorded amounts. Advisors read it too, but only over their own sales.
func (s *Service) Finance(ctx context.Context, actor auth.Actor, f Filter) (*Finance, error) {
	f, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	out := &Finance{Period: w.period(), GeneratedAt: now}
	err = run(
		func() (err error) {
			out.Pipeline, out.Completed, err = loadPipelineAndCompleted(ctx, s.db, w, f)
			return
		},
		func() (err error) { out.Cash, err = loadCash(ctx, s.db, w, f); return },
		func() (err error) { out.Invoices, err = loadInvoices(ctx, s.db, w, f); return },
		func() (err error) { out.Monthly, err = loadMoneyMonths(ctx, s.db, w, f); return },
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Advisors compares the sales team. Managers and Admins only.
func (s *Service) Advisors(ctx context.Context, actor auth.Actor, f Filter) (*Advisors, error) {
	if actor.Role != auth.RoleSystemAdmin && actor.Role != auth.RoleSalesManager {
		return nil, errForbidden
	}
	f, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	var (
		rows    []AdvisorRow
		overdue map[uuid.UUID]int
	)
	err = run(
		func() (err error) { rows, err = loadAdvisorRows(ctx, s.db, w, f); return },
		func() (err error) { overdue, err = loadOverdueByAdvisor(ctx, s.db, f, now); return },
	)
	if err != nil {
		return nil, err
	}
	items := make([]AdvisorRow, 0, len(rows))
	for _, r := range rows {
		r.OverdueTasks = overdue[r.AdvisorID]
		// An advisor with no sale in play and nothing in the period would only add an empty row.
		if r.ActiveSales == 0 && r.CreatedInPeriod == 0 && r.CompletedInPeriod == 0 && r.CancelledInPeriod == 0 {
			continue
		}
		items = append(items, r)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ActiveSales != items[j].ActiveSales {
			return items[i].ActiveSales > items[j].ActiveSales
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return &Advisors{Period: w.period(), GeneratedAt: now, Items: items}, nil
}

// System is the Admin-only tab about the platform. It ignores the sale filters and only uses the period.
func (s *Service) System(ctx context.Context, actor auth.Actor, f Filter) (*System, error) {
	if actor.Role != auth.RoleSystemAdmin {
		return nil, errForbidden
	}
	_, w, now, err := s.prepare(actor, f)
	if err != nil {
		return nil, err
	}
	out := &System{Period: w.period(), GeneratedAt: now}
	err = run(
		func() (err error) { out.Staff, err = loadStaffStats(ctx, s.db, now); return },
		func() (err error) { out.Notifications, err = loadNotificationStats(ctx, s.db, w); return },
		func() (err error) { out.Security, err = loadSecurityStats(ctx, s.db, w); return },
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}
