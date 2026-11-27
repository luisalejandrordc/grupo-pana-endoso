package dashboard_test

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/dashboard"
	"endosoflow/internal/documents"
	"endosoflow/internal/notifications"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"endosoflow/internal/testkit"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fx builds the real sales and tasks services over the test database, creates sales through them, and reads
// them back through the dashboard. Nothing is inserted by hand, so the numbers prove the whole chain.
type fx struct {
	*testkit.Env
	sales   *sales.Service
	tasks   *tasks.Service
	dash    *dashboard.Service
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
	docs := documents.NewService(documents.Deps{DB: env.Pool, Store: store, Sales: sales.Gate{}, Logger: env.Logger})
	nts := notifications.Writer{}
	f := &fx{
		Env:   env,
		sales: sales.NewService(sales.Deps{DB: env.Pool, Images: store, Documents: docs, DocumentStore: store, Notifier: nts, Logger: env.Logger}),
		tasks: tasks.NewService(tasks.Deps{DB: env.Pool, Sales: sales.Gate{}, Documents: docs, Notifier: nts, Logger: env.Logger}),
		// Three days ahead, so a sale whose first task nobody started counts as stalled (the limit is 48 hours).
		dash: dashboard.NewService(dashboard.Deps{DB: env.Pool, Logger: env.Logger, Now: func() time.Time { return time.Now().Add(72 * time.Hour) }}),
	}
	f.admin = env.Employee(auth.RoleSystemAdmin, false)
	f.manager = env.Employee(auth.RoleSalesManager, false)
	f.advisor = env.Employee(auth.RoleSalesAdvisor, false)
	f.other = env.Employee(auth.RoleSalesAdvisor, false)
	f.vehicle = env.Vehicle(f.admin.ID)
	return f
}

func (f *fx) newSale(advisor testkit.Employee) *sales.Sale {
	f.T.Helper()
	customer := f.Customer(f.manager.ID, false)
	s, err := f.sales.Create(f.Ctx, f.manager.Actor(), sales.CreateInput{
		CustomerID: customer, VersionID: f.vehicle.VersionID, ColorID: f.vehicle.ColorID, Temperature: "hot", AdvisorID: &advisor.ID,
	})
	if err != nil {
		f.T.Fatalf("create sale: %v", err)
	}
	return s
}

func digits(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + rand.IntN(10))
	}
	return string(b)
}

func (f *fx) validData(n int) string {
	d := time.Now().In(time.FixedZone("Lima", -5*3600)).Format("2006-01-02")
	switch n {
	case 1:
		return `{"initialToyotaAmount":"30000","initialCommission":"2500","accessoryAmount":"1200.50","releaseProcessingAmount":"800"}`
	case 2:
		return fmt.Sprintf(`{"signerDocumentType":"DNI","signerDocumentNumber":"%s","signerFirstName":"Ana","signerLastName":"Rojas","pepStatus":"no_pep","ddcRegime":"general"}`, digits(8))
	case 3:
		return fmt.Sprintf(`{"vehicleSeries":"S%s","vin":"JT4RN81A0M5%s","finalToyotaAmount":"31000"}`, digits(8), digits(6))
	case 4:
		return fmt.Sprintf(`{"bank":"bcp","paymentDate":"%s","transactionNumber":"T%s","depositedAmount":"34500.50"}`, d, digits(8))
	case 5:
		return fmt.Sprintf(`{"bank":"bbva","paymentDate":"%s","transactionNumber":"T%s","depositedAmount":"31000"}`, d, digits(8))
	case 6:
		return fmt.Sprintf(`{"toyota":{"number":"F001-%s","date":"%s","amount":"31000"},"commission":{"number":"F002-%s","date":"%s","amount":"1500"},
			"accessories":{"number":"F003-%s","date":"%s","amount":"1200.50"},"releaseProcessing":{"number":"F004-%s","date":"%s","amount":"800"}}`,
			digits(6), d, digits(6), d, digits(6), d, digits(6), d)
	case 8:
		return fmt.Sprintf(`{"damNumber":"118-2026-%s","customsOffice":"callao_maritima","controlChannel":"green"}`, digits(8))
	case 9:
		return `{"personInCharge":"Juan Pérez"}`
	case 10:
		return fmt.Sprintf(`{"licensePlate":"A%s","registrationDate":"%s"}`, digits(5), d)
	case 11:
		return fmt.Sprintf(`{"pdsTransferDate":"%s"}`, d)
	case 12:
		return fmt.Sprintf(`{"serviceOrderNumber":"OS-%s","equipmentCompletionDate":"%s"}`, digits(5), d)
	case 13:
		return fmt.Sprintf(`{"deliveryDate":"%s"}`, d)
	}
	return `{}`
}

// advance completes tasks 1 to n as the sale's advisor.
func (f *fx) advance(sale uuid.UUID, advisor testkit.Employee, n int) {
	f.T.Helper()
	for i := 1; i <= n; i++ {
		if _, err := f.tasks.Start(f.Ctx, advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("start %d: %v", i, err)
		}
		if i != 7 {
			d, err := f.tasks.Get(f.Ctx, advisor.Actor(), sale, i)
			if err != nil {
				f.T.Fatalf("get %d: %v", i, err)
			}
			v := d.Version
			if _, err := f.tasks.SaveData(f.Ctx, advisor.Actor(), sale, i, tasks.SaveInput{Version: &v, Data: []byte(f.validData(i))}); err != nil {
				f.T.Fatalf("save %d: %v", i, err)
			}
		}
		f.FillRequiredDocuments(sale, i, advisor.ID)
		if _, err := f.tasks.Complete(f.Ctx, advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("complete %d: %v", i, err)
		}
	}
}

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// scenario gives the advisor three sales: one delivered (all 13 tasks), one cancelled at task 1, and one
// still waiting for task 1 to start. The other advisor has nothing.
func (f *fx) scenario() {
	f.T.Helper()
	done := f.newSale(f.advisor)
	f.advance(done.ID, f.advisor, 13)
	cancelled := f.newSale(f.advisor)
	if _, err := f.sales.Cancel(f.Ctx, f.advisor.Actor(), cancelled.ID, "customer_withdrew", "El cliente desistió"); err != nil {
		f.T.Fatalf("cancel: %v", err)
	}
	f.newSale(f.advisor)
}

func TestSummaryCountsTheAdvisorsOwnSales(t *testing.T) {
	f := newFx(t)
	f.scenario()
	got, err := f.dash.Summary(f.Ctx, f.advisor.Actor(), dashboard.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	k := got.Kpis
	if k.ActiveSales != 1 || k.CreatedInPeriod != 3 || k.CompletedInPeriod != 1 || k.CancelledInPeriod != 1 {
		t.Fatalf("kpis = %+v", k)
	}
	if k.ClosingRate == nil || *k.ClosingRate != 50 {
		t.Fatalf("closing rate = %v, want 50", k.ClosingRate)
	}
	if k.StalledTasks != 1 || got.Attention.Total != 1 || got.Attention.Items[0].Kind != dashboard.KindStalled || got.Attention.Items[0].TaskNumber != 1 {
		t.Fatalf("the sale whose first task nobody started should be stalled: %+v", got.Attention)
	}
	if len(got.Milestones) != 6 || got.Milestones[0].ActiveSales != 1 {
		t.Fatalf("milestones = %+v", got.Milestones)
	}
	var created int
	for _, p := range got.Trend {
		created += p.Created
	}
	if created != 3 {
		t.Fatalf("the trend should add up to the 3 sales created, got %d (%+v)", created, got.Trend)
	}
}

func TestAnAdvisorCannotWidenTheirScope(t *testing.T) {
	f := newFx(t)
	f.scenario()
	// The other advisor asks for the first advisor's sales and still gets only their own, which are none.
	got, err := f.dash.Summary(f.Ctx, f.other.Actor(), dashboard.Filter{AdvisorID: &f.advisor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Kpis.ActiveSales != 0 || got.Kpis.CreatedInPeriod != 0 {
		t.Fatalf("an advisor saw another advisor's sales: %+v", got.Kpis)
	}
}

func TestManagerFiltersByAdvisorAndVehicle(t *testing.T) {
	f := newFx(t)
	f.scenario()
	got, err := f.dash.Summary(f.Ctx, f.manager.Actor(), dashboard.Filter{AdvisorID: &f.advisor.ID, ModelID: &f.vehicle.ModelID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Kpis.CreatedInPeriod != 3 {
		t.Fatalf("created = %d, want 3", got.Kpis.CreatedInPeriod)
	}
	none := uuid.New()
	got, err = f.dash.Summary(f.Ctx, f.manager.Actor(), dashboard.Filter{AdvisorID: &f.advisor.ID, ModelID: &none})
	if err != nil || got.Kpis.CreatedInPeriod != 0 {
		t.Fatalf("an unknown model should match nothing: %+v, %v", got, err)
	}
}

func TestFinanceCountsOnlyConfirmedAmounts(t *testing.T) {
	f := newFx(t)
	f.scenario()
	got, err := f.dash.Finance(f.Ctx, f.advisor.Actor(), dashboard.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	c := got.Completed
	// Quoted 30000 + 1200.50 + 800 + 2500 = 34500.50; the final commission is 34500.50 - 31000 - 1200.50 - 800 = 1500.
	if c.Sales != 1 || c.QuotedValue != "34500.50" || c.Commission != "1500.00" || c.Accessories != "1200.50" ||
		c.ReleaseProcessing != "800.00" || c.ToyotaAmount != "31000.00" || c.AverageCommission != "1500.00" {
		t.Fatalf("completed money = %+v", c)
	}
	if got.Cash.MoneyIn != "34500.50" || got.Cash.MoneyOut != "31000.00" || got.Cash.Net != "3500.50" || got.Cash.PaymentMismatches != 0 {
		t.Fatalf("cash = %+v", got.Cash)
	}
	// The sale still waiting for task 1 has no confirmed amounts, so the pipeline is empty.
	if got.Pipeline.Sales != 0 || got.Pipeline.QuotedValue != "0.00" {
		t.Fatalf("pipeline = %+v", got.Pipeline)
	}
	if len(got.Invoices) != 4 || got.Invoices[0].Kind != "toyota" || got.Invoices[1].Amount != "1500.00" || got.Invoices[1].Mismatches != 0 {
		t.Fatalf("invoices = %+v", got.Invoices)
	}
	var commission string
	for _, m := range got.Monthly {
		if m.Commission != "0.00" {
			commission = m.Commission
		}
	}
	if commission != "1500.00" {
		t.Fatalf("monthly = %+v", got.Monthly)
	}
}

func TestProcessReportsTasksCycleTimeAndCancellations(t *testing.T) {
	f := newFx(t)
	f.scenario()
	got, err := f.dash.Process(f.Ctx, f.advisor.Actor(), dashboard.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 13 {
		t.Fatalf("tasks = %d, want 13", len(got.Tasks))
	}
	if got.Tasks[0].CompletedInPeriod != 1 || got.Tasks[0].ActiveSales != 1 || got.Tasks[12].CompletedInPeriod != 1 {
		t.Fatalf("task 1 = %+v, task 13 = %+v", got.Tasks[0], got.Tasks[12])
	}
	if got.CycleTime.SampleSize != 1 || got.CycleTime.MedianDays == nil {
		t.Fatalf("cycle time = %+v", got.CycleTime)
	}
	if got.Cancellations.Total != 1 || got.Cancellations.ByCategory[0].Category != "customer_withdrew" ||
		len(got.Cancellations.ByTask) != 1 || got.Cancellations.ByTask[0].Number != 1 {
		t.Fatalf("cancellations = %+v", got.Cancellations)
	}
}

func TestMarketGroupsTheSalesCreatedInThePeriod(t *testing.T) {
	f := newFx(t)
	f.scenario()
	got, err := f.dash.Market(f.Ctx, f.advisor.Actor(), dashboard.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].Total != 3 || got.Models[0].Completed != 1 || got.Models[0].Cancelled != 1 || got.Models[0].Active != 1 {
		t.Fatalf("models = %+v", got.Models)
	}
	if len(got.Temperatures) != 3 || got.Temperatures[0].Temperature != "hot" || got.Temperatures[0].Total != 3 ||
		got.Temperatures[0].ClosingRate == nil || *got.Temperatures[0].ClosingRate != 50 {
		t.Fatalf("temperatures = %+v", got.Temperatures)
	}
	if got.Temperatures[1].Total != 0 || got.Temperatures[1].ClosingRate != nil {
		t.Fatalf("an unused temperature should be empty: %+v", got.Temperatures[1])
	}
}

func TestAdvisorsAndSystemTabsAreClosedToTheWrongRoles(t *testing.T) {
	f := newFx(t)
	f.scenario()
	ctx := context.Background()
	if _, err := f.dash.Advisors(ctx, f.advisor.Actor(), dashboard.Filter{}); code(err) != "FORBIDDEN" {
		t.Fatalf("an advisor reading the advisors tab: %v", err)
	}
	if _, err := f.dash.System(ctx, f.manager.Actor(), dashboard.Filter{}); code(err) != "FORBIDDEN" {
		t.Fatalf("a manager reading the system tab: %v", err)
	}
	got, err := f.dash.Advisors(ctx, f.manager.Actor(), dashboard.Filter{AdvisorID: &f.advisor.ID})
	if err != nil || len(got.Items) != 1 {
		t.Fatalf("advisors = %+v, %v", got, err)
	}
	row := got.Items[0]
	if row.AdvisorID != f.advisor.ID || row.ActiveSales != 1 || row.CreatedInPeriod != 3 || row.CompletedInPeriod != 1 ||
		row.CancelledInPeriod != 1 || row.Commission != "1500.00" || row.PipelineValue != "0.00" {
		t.Fatalf("advisor row = %+v", row)
	}
	sys, err := f.dash.System(ctx, f.admin.Actor(), dashboard.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if sys.Staff.Admins < 1 || sys.Staff.Managers < 1 || sys.Staff.Advisors < 2 {
		t.Fatalf("staff = %+v", sys.Staff)
	}
}

func TestABadPeriodIsRejected(t *testing.T) {
	f := newFx(t)
	_, err := f.dash.Summary(f.Ctx, f.manager.Actor(), dashboard.Filter{From: "2026-12-01", To: "2026-11-01"})
	if code(err) != "INVALID_QUERY" {
		t.Fatalf("got %v", err)
	}
}
