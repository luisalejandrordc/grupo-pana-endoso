package sales_test

import (
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/documents"
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
	store   *storage.Memory
	sales   *sales.Service
	tasks   *tasks.Service
	docs    *documents.Service
	clock   *clock
	admin   testkit.Employee
	manager testkit.Employee
	advisor testkit.Employee
	other   testkit.Employee
	vehicle testkit.Vehicle
}

func newFx(t *testing.T) *fx {
	t.Helper()
	env := testkit.New(t)
	clk := &clock{t: time.Now()}
	store := storage.NewMemory("https://img.test")
	docs := documents.NewService(documents.Deps{DB: env.Pool, Store: store, Sales: sales.Gate{}, Logger: env.Logger, Now: clk.Now})
	f := &fx{
		Env: env, clock: clk, store: store, docs: docs,
		sales: sales.NewService(sales.Deps{DB: env.Pool, Images: store, Documents: docs, DocumentStore: store, Logger: env.Logger, Now: clk.Now}),
		tasks: tasks.NewService(tasks.Deps{DB: env.Pool, Sales: sales.Gate{}, Documents: docs, Logger: env.Logger, Now: clk.Now}),
	}
	f.admin = env.Employee(auth.RoleSystemAdmin, false)
	f.manager = env.Employee(auth.RoleSalesManager, false)
	f.advisor = env.Employee(auth.RoleSalesAdvisor, false)
	f.other = env.Employee(auth.RoleSalesAdvisor, false)
	f.vehicle = env.Vehicle(f.admin.ID)
	return f
}

func (f *fx) input(customer uuid.UUID, advisor *uuid.UUID) sales.CreateInput {
	return sales.CreateInput{CustomerID: customer, VersionID: f.vehicle.VersionID, ColorID: f.vehicle.ColorID, Temperature: "hot", AdvisorID: advisor}
}

// newSale creates a sale for the advisor, made by the manager, for a fresh Diplomático.
func (f *fx) newSale(advisor testkit.Employee) *sales.Sale {
	f.T.Helper()
	customer := f.Customer(f.manager.ID, false)
	s, err := f.sales.Create(f.Ctx, f.manager.Actor(), f.input(customer, &advisor.ID))
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

func (f *fx) today() string {
	return f.clock.Now().In(time.FixedZone("Lima", -5*3600)).Format("2006-01-02")
}

func (f *fx) validData(n int) string {
	d := f.today()
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

func (f *fx) taskVersion(actor testkit.Employee, sale uuid.UUID, n int) int {
	d, err := f.tasks.Get(f.Ctx, actor.Actor(), sale, n)
	if err != nil {
		f.T.Fatalf("get task: %v", err)
	}
	return d.Version
}

// advance completes tasks 1 to n as the sale's advisor.
func (f *fx) advance(sale uuid.UUID, advisor testkit.Employee, n int) {
	f.T.Helper()
	for i := 1; i <= n; i++ {
		if _, err := f.tasks.Start(f.Ctx, advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("start %d: %v", i, err)
		}
		if i != 7 {
			v := f.taskVersion(advisor, sale, i)
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

func (f *fx) auditCount(sale uuid.UUID, entityType, action string) int {
	f.T.Helper()
	var n int
	if err := f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM audit_logs WHERE sale_id = $1 AND entity_type = $2::text::audit_entity_type AND action = $3::text::audit_action`,
		sale, entityType, action).Scan(&n); err != nil {
		f.T.Fatal(err)
	}
	return n
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func tasksSave(version int, data string) tasks.SaveInput {
	return tasks.SaveInput{Version: &version, Data: []byte(data)}
}
