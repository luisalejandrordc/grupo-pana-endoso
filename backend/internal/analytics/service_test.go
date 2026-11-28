package analytics_test

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/analytics"
	"endosoflow/internal/auth"
	"endosoflow/internal/notifications"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/sales"
	"endosoflow/internal/testkit"
	"errors"
	"testing"
	"time"
)

type fx struct {
	*testkit.Env
	svc     *analytics.Service
	sales   *sales.Service
	admin   testkit.Employee
	manager testkit.Employee
	advisor testkit.Employee
	other   testkit.Employee
	vehicle testkit.Vehicle
}

func newFx(t *testing.T) *fx {
	t.Helper()
	env := testkit.New(t)
	store := storage.NewMemory("https://img.test")
	f := &fx{
		Env:   env,
		svc:   analytics.NewService(analytics.Deps{DB: env.Pool, Logger: env.Logger}),
		sales: sales.NewService(sales.Deps{DB: env.Pool, Images: store, DocumentStore: store, Notifier: notifications.Writer{}, Logger: env.Logger}),
	}
	f.admin = env.Employee(auth.RoleSystemAdmin, false)
	f.manager = env.Employee(auth.RoleSalesManager, false)
	f.advisor = env.Employee(auth.RoleSalesAdvisor, false)
	f.other = env.Employee(auth.RoleSalesAdvisor, false)
	f.vehicle = env.Vehicle(f.admin.ID)
	return f
}

func (f *fx) newSale(advisor testkit.Employee, temperature string) *sales.Sale {
	f.T.Helper()
	customer := f.Customer(f.manager.ID, false)
	s, err := f.sales.Create(f.Ctx, f.manager.Actor(), sales.CreateInput{
		CustomerID: customer, VersionID: f.vehicle.VersionID, ColorID: f.vehicle.ColorID,
		Temperature: temperature, AdvisorID: &advisor.ID,
	})
	if err != nil {
		f.T.Fatalf("create sale: %v", err)
	}
	return s
}

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestAnAdvisorOnlySeesTheirOwnSales(t *testing.T) {
	f := newFx(t)
	f.newSale(f.advisor, "hot")
	f.newSale(f.other, "cold")
	f.newSale(f.other, "cold")

	// Even when the Advisor filters by someone else's id they still get only their own sales.
	got, err := f.svc.Overview(f.Ctx, f.advisor.Actor(), analytics.Filter{AdvisorID: &f.other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.ActiveSales != 1 || got.Period.Created.Value != 1 {
		t.Fatalf("advisor sees %d active, %d created; want 1 and 1", got.Snapshot.ActiveSales, got.Period.Created.Value)
	}
	if got.Meta.Scope != "own" {
		t.Fatalf("scope = %s, want own", got.Meta.Scope)
	}
	if got.Money.Internal != nil {
		t.Fatal("an Advisor must never receive the internal money")
	}

	// A Manager filtering by an advisor sees that advisor's sales, and the internal money.
	m, err := f.svc.Overview(f.Ctx, f.manager.Actor(), analytics.Filter{AdvisorID: &f.other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if m.Snapshot.ActiveSales != 2 || m.Meta.Scope != "global" || m.Money.Internal == nil {
		t.Fatalf("manager sees %d active, scope %s, internal %v", m.Snapshot.ActiveSales, m.Meta.Scope, m.Money.Internal != nil)
	}
}

func TestRestrictedTabsRefuseTheWrongRole(t *testing.T) {
	f := newFx(t)
	if _, err := f.svc.Team(f.Ctx, f.advisor.Actor(), analytics.Filter{}); code(err) != "FORBIDDEN" {
		t.Errorf("an Advisor asking for the team tab: %v", err)
	}
	if _, err := f.svc.System(f.Ctx, f.manager.Actor(), analytics.Filter{}); code(err) != "FORBIDDEN" {
		t.Errorf("a Manager asking for the system tab: %v", err)
	}
}

func TestCommercialHidesCommissionFromAdvisors(t *testing.T) {
	f := newFx(t)
	f.newSale(f.advisor, "hot")
	own, err := f.svc.Commercial(f.Ctx, f.advisor.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if own.PriceComposition != nil {
		t.Error("price composition is Manager and Admin only")
	}
	for _, v := range own.ValueTrend {
		if v.Commission != nil {
			t.Fatal("trend commission is Manager and Admin only")
		}
	}
	for _, v := range own.Vehicles {
		if v.Commission != nil {
			t.Fatal("vehicle commission is Manager and Admin only")
		}
	}
	mgr, err := f.svc.Commercial(f.Ctx, f.manager.Actor(), analytics.Filter{AdvisorID: &f.advisor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if mgr.PriceComposition == nil {
		t.Error("a Manager must receive the price composition")
	}
}

func TestPipelineValueUsesTheReferencePriceBeforeAQuote(t *testing.T) {
	f := newFx(t)
	f.newSale(f.advisor, "hot")
	got, err := f.svc.Overview(f.Ctx, f.advisor.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	v := got.Money.PipelineValue
	if v.Amount != "35000.00" || v.SaleCount != 1 || v.EstimatedCount != 1 {
		t.Fatalf("pipeline value = %+v, want 35000.00 over 1 sale, 1 estimated", v)
	}
	if got.Money.PendingCollection.SaleCount != 0 {
		t.Fatalf("a sale without a quote cannot be pending collection: %+v", got.Money.PendingCollection)
	}
}

func TestCancelledSalesCountAsLostValueAndLowerTheCloseRate(t *testing.T) {
	f := newFx(t)
	f.newSale(f.advisor, "hot")
	drop := f.newSale(f.advisor, "hot")
	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), drop.ID, "customer_withdrew", "El cliente desistió."); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, err := f.svc.Overview(f.Ctx, f.manager.Actor(), analytics.Filter{AdvisorID: &f.advisor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.ActiveSales != 1 || got.Period.Created.Value != 2 || got.Period.Cancelled.Value != 1 {
		t.Fatalf("snapshot/period = %+v / %+v", got.Snapshot, got.Period)
	}
	if r := got.Period.CloseRate; r.Numerator != 0 || r.Denominator != 1 || r.Value == nil || *r.Value != 0 {
		t.Fatalf("close rate = %+v, want 0 of 1", r)
	}
	if got.Money.LostValue.Amount != "35000.00" || got.Money.LostValue.SaleCount != 1 {
		t.Fatalf("lost value = %+v", got.Money.LostValue)
	}
	com, err := f.svc.Commercial(f.Ctx, f.manager.Actor(), analytics.Filter{AdvisorID: &f.advisor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if com.Cancellations.Total.SaleCount != 1 || len(com.Cancellations.ByCategory) != 1 || com.Cancellations.ByCategory[0].Category != "customer_withdrew" {
		t.Fatalf("cancellations = %+v", com.Cancellations)
	}
	if len(com.Cancellations.ByTask) != 1 || com.Cancellations.ByTask[0].TaskNumber != 1 {
		t.Fatalf("cancellations by task = %+v", com.Cancellations.ByTask)
	}
}

func TestAnIdleSaleIsFlaggedAfterTwentyFourHours(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor, "warm")
	soon := analytics.NewService(analytics.Deps{DB: f.Pool, Logger: f.Logger, Now: func() time.Time { return time.Now().Add(2 * time.Hour) }})
	late := analytics.NewService(analytics.Deps{DB: f.Pool, Logger: f.Logger, Now: func() time.Time { return time.Now().Add(25 * time.Hour) }})

	got, err := soon.Overview(f.Ctx, f.advisor.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.Idle != 0 || got.Attention.Total != 0 {
		t.Fatalf("after 2 hours: %+v", got.Snapshot)
	}
	got, err = late.Overview(f.Ctx, f.advisor.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.Idle != 1 || got.Attention.Total != 1 || len(got.Attention.Items) != 1 {
		t.Fatalf("after 25 hours: %+v, attention %+v", got.Snapshot, got.Attention)
	}
	item := got.Attention.Items[0]
	if item.Reason != "idle" || item.TaskNumber != 1 || item.SaleID != s.ID.String() || item.Hours < 24 {
		t.Fatalf("attention item = %+v", item)
	}
	if got.Pipeline[0].Idle != 1 || got.Pipeline[0].Active != 1 {
		t.Fatalf("milestone 1 = %+v", got.Pipeline[0])
	}
}

func TestAnEmptyScopeReturnsZerosAndCompleteShapes(t *testing.T) {
	f := newFx(t) // f.other has no sales
	got, err := f.svc.Overview(f.Ctx, f.other.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.ActiveSales != 0 || got.Period.CloseRate.Value != nil || got.Period.CycleTime.MedianHours != nil {
		t.Fatalf("empty scope: %+v %+v", got.Snapshot, got.Period)
	}
	if got.Money.PipelineValue.Amount != "0.00" || got.Money.AverageTicket.Value != nil {
		t.Fatalf("empty money: %+v", got.Money)
	}
	if len(got.Pipeline) != 6 || len(got.Trend) == 0 || got.Attention.Items == nil {
		t.Fatalf("shapes: %d milestones, %d buckets", len(got.Pipeline), len(got.Trend))
	}
	proc, err := f.svc.Process(f.Ctx, f.other.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(proc.Tasks) != 13 || proc.OnTimeRate.Value != nil {
		t.Fatalf("process: %d tasks, on-time %+v", len(proc.Tasks), proc.OnTimeRate)
	}
	withDeadline := map[int]bool{2: true, 3: true, 7: true, 8: true, 9: true, 10: true}
	for _, task := range proc.Tasks {
		if (task.Deadlines != nil) != withDeadline[task.TaskNumber] {
			t.Errorf("task %d: deadlines present = %v", task.TaskNumber, task.Deadlines != nil)
		}
	}
	com, err := f.svc.Commercial(f.Ctx, f.other.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(com.Temperatures) != 3 || com.Temperatures[0].Temperature != "hot" || com.Temperatures[2].Temperature != "cold" {
		t.Fatalf("temperatures = %+v", com.Temperatures)
	}
}

// The remaining tabs and every grouping run against the real schema, so a mistake in any query shows up here.
func TestEveryQueryRunsForAManagerAndAnAdmin(t *testing.T) {
	f := newFx(t)
	f.newSale(f.advisor, "hot")
	ctx := context.Background()
	for _, g := range []string{"model", "version", "color", "category"} {
		if _, err := f.svc.Commercial(ctx, f.manager.Actor(), analytics.Filter{GroupBy: g}); err != nil {
			t.Errorf("commercial by %s: %v", g, err)
		}
	}
	team, err := f.svc.Team(ctx, f.manager.Actor(), analytics.Filter{})
	if err != nil {
		t.Fatalf("team: %v", err)
	}
	found := false
	for _, a := range team.Advisors {
		if a.AdvisorID == f.advisor.ID.String() {
			found = a.Active == 1 && a.Created == 1
		}
	}
	if !found {
		t.Errorf("the advisor with one new sale is missing from the team table: %+v", team.Advisors)
	}
	if _, err := f.svc.System(ctx, f.admin.Actor(), analytics.Filter{}); err != nil {
		t.Fatalf("system: %v", err)
	}
	// A month-long period uses daily buckets, a two-year one monthly buckets.
	from := time.Now().AddDate(0, 0, -20)
	if _, err := f.svc.Overview(ctx, f.manager.Actor(), analytics.Filter{From: &from}); err != nil {
		t.Fatalf("daily buckets: %v", err)
	}
	from = time.Now().AddDate(-1, 0, 0)
	if _, err := f.svc.Overview(ctx, f.manager.Actor(), analytics.Filter{From: &from}); err != nil {
		t.Fatalf("monthly buckets: %v", err)
	}
}
