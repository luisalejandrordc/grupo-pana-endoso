package tasks_test

import (
	"encoding/json"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"endosoflow/internal/testkit"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// clock lets a test move time forward to exercise the SLA rules.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fx struct {
	*testkit.Env
	sales   *sales.Service
	tasks   *tasks.Service
	clock   *clock
	admin   testkit.Employee
	manager testkit.Employee
	advisor testkit.Employee
	other   testkit.Employee // another advisor
	vehicle testkit.Vehicle
}

func newFx(t *testing.T) *fx {
	t.Helper()
	env := testkit.New(t)
	clk := &clock{t: time.Now()}
	f := &fx{
		Env: env, clock: clk,
		sales: sales.NewService(sales.Deps{DB: env.Pool, Store: storage.NewMemory("https://img.test"), Logger: env.Logger}),
		tasks: tasks.NewService(tasks.Deps{DB: env.Pool, Logger: env.Logger, Now: clk.Now}),
	}
	f.admin = env.Employee(auth.RoleSystemAdmin, false)
	f.manager = env.Employee(auth.RoleSalesManager, false)
	f.advisor = env.Employee(auth.RoleSalesAdvisor, false)
	f.other = env.Employee(auth.RoleSalesAdvisor, false)
	f.vehicle = env.Vehicle(f.admin.ID)
	return f
}

// newSale creates a sale for the advisor through the real service.
func (f *fx) newSale(advisor testkit.Employee) uuid.UUID {
	f.T.Helper()
	customer := f.Customer(f.manager.ID, false)
	sale, err := f.sales.Create(f.Ctx, f.manager.Actor(), sales.CreateInput{
		CustomerID: customer, VersionID: f.vehicle.VersionID,
		ColorID: f.vehicle.ColorID, Temperature: "hot", AdvisorID: &advisor.ID,
	})
	if err != nil {
		f.T.Fatalf("create sale: %v", err)
	}
	return sale.ID
}

func (f *fx) detail(actor testkit.Employee, sale uuid.UUID, n int) *tasks.Detail {
	f.T.Helper()
	d, err := f.tasks.Get(f.Ctx, actor.Actor(), sale, n)
	if err != nil {
		f.T.Fatalf("get task %d: %v", n, err)
	}
	return d
}

func (f *fx) start(actor testkit.Employee, sale uuid.UUID, n int) error {
	_, err := f.tasks.Start(f.Ctx, actor.Actor(), sale, n)
	return err
}

func (f *fx) complete(actor testkit.Employee, sale uuid.UUID, n int) error {
	_, err := f.tasks.Complete(f.Ctx, actor.Actor(), sale, n)
	return err
}

// save sends fields for task n using the current version of the task.
func (f *fx) save(actor testkit.Employee, sale uuid.UUID, n int, data string) (*tasks.Detail, error) {
	return f.saveWithReason(actor, sale, n, data, "")
}

func (f *fx) saveWithReason(actor testkit.Employee, sale uuid.UUID, n int, data, reason string) (*tasks.Detail, error) {
	f.T.Helper()
	version := f.detail(actor, sale, n).Version
	return f.tasks.SaveData(f.Ctx, actor.Actor(), sale, n, tasks.SaveInput{Version: &version, Reason: reason, Data: []byte(data)})
}

func (f *fx) mustSave(actor testkit.Employee, sale uuid.UUID, n int, data string) *tasks.Detail {
	f.T.Helper()
	d, err := f.save(actor, sale, n, data)
	if err != nil {
		f.T.Fatalf("save task %d: %v", n, err)
	}
	return d
}

func digits(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + rand.IntN(10))
	}
	return string(b)
}

func today(f *fx) string {
	return f.clock.Now().In(time.FixedZone("Lima", -5*3600)).Format("2006-01-02")
}

// validData returns complete, valid data for task n of a sale with the standard amounts (initial Toyota 30000,
// commission 2500, accessories 1200.50, release 800; final Toyota 31000, so the final commission is 1500).
func (f *fx) validData(n int) string {
	d := today(f)
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
	case 7:
		return `{}`
	case 8:
		return fmt.Sprintf(`{"damNumber":"118-2026-%s","customsOffice":"callao_maritima","controlChannel":"green"}`, digits(8))
	case 9:
		return `{"personInCharge":"Juan Pérez"}`
	case 10:
		return fmt.Sprintf(`{"licensePlate":"A%s","registrationDate":"%s"}`, strings.ToUpper(digits(5)), d)
	case 11:
		return fmt.Sprintf(`{"pdsTransferDate":"%s"}`, d)
	case 12:
		return fmt.Sprintf(`{"serviceOrderNumber":"OS-%s","equipmentCompletionDate":"%s"}`, digits(5), d)
	case 13:
		return fmt.Sprintf(`{"deliveryDate":"%s"}`, d)
	}
	return `{}`
}

// run starts, fills and completes task n.
func (f *fx) run(actor testkit.Employee, sale uuid.UUID, n int) {
	f.T.Helper()
	if err := f.start(actor, sale, n); err != nil {
		f.T.Fatalf("start %d: %v", n, err)
	}
	if n != 7 {
		f.mustSave(actor, sale, n, f.validData(n))
	}
	f.FillRequiredDocuments(sale, n, actor.ID)
	if err := f.complete(actor, sale, n); err != nil {
		f.T.Fatalf("complete %d: %v", n, err)
	}
}

// runThrough completes tasks 1 to n.
func (f *fx) runThrough(actor testkit.Employee, sale uuid.UUID, n int) {
	f.T.Helper()
	for i := 1; i <= n; i++ {
		f.run(actor, sale, i)
	}
}

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("got error %v (code %q), want code %q", err, got, want)
	}
}

func fieldsOf(err error) map[string]string {
	out := map[string]string{}
	var e *apperr.Error
	if errors.As(err, &e) {
		for _, fl := range e.Fields {
			out[fl.Field] = fl.Code
		}
	}
	return out
}

func (f *fx) auditCount(entityID uuid.UUID, action string) int {
	f.T.Helper()
	var n int
	if err := f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = $2::text::audit_action`, entityID, action).Scan(&n); err != nil {
		f.T.Fatal(err)
	}
	return n
}

func (f *fx) taskID(sale uuid.UUID, n int) uuid.UUID {
	f.T.Helper()
	var id uuid.UUID
	if err := f.Pool.QueryRow(f.Ctx, `SELECT id FROM task_instances WHERE sale_id = $1 AND task_number = $2`, sale, n).Scan(&id); err != nil {
		f.T.Fatal(err)
	}
	return id
}

func (f *fx) notifications(sale uuid.UUID) map[string]int {
	f.T.Helper()
	rows, err := f.Pool.Query(f.Ctx, `SELECT notification_type::text, count(*) FROM notification_logs WHERE sale_id = $1 GROUP BY 1`, sale)
	if err != nil {
		f.T.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var c int
		if err := rows.Scan(&k, &c); err != nil {
			f.T.Fatal(err)
		}
		out[k] = c
	}
	return out
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(v)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func salesInput(f *fx, customer uuid.UUID) sales.CreateInput {
	return sales.CreateInput{CustomerID: customer, VersionID: f.vehicle.VersionID, ColorID: f.vehicle.ColorID, Temperature: "warm", AdvisorID: &f.advisor.ID}
}
