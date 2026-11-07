package sales

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The flow tests run the sales, tasks and documents services together against a migrated database. Run
// `make db-verify` once, set TEST_DATABASE_URL and run `make test`; without the variable they are skipped.
// Each test creates its own employees, customer, catalog entries and sales, and removes them afterwards.

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func (c *clock) Advance(d time.Duration) { c.Set(c.Now().Add(d)) }

type testEmployee struct {
	ID   uuid.UUID
	Role auth.Role
}

func (e testEmployee) actor() auth.Actor { return auth.Actor{ID: e.ID, Role: e.Role} }

type catalog struct{ ModelID, VersionID, ColorID uuid.UUID }

type flowEnv struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	store     *storage.Memory
	clock     *clock
	sales     *Service
	tasks     *tasks.Service
	docs      *documents.Service
	customers *customers.Service

	admin, manager, advisor, other testEmployee
}

func newFlowEnv(t *testing.T) *flowEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	clk := &clock{t: time.Now().UTC().Truncate(time.Second)}
	store := storage.NewMemory("https://files.test")
	gate := Gate{}
	e := &flowEnv{t: t, ctx: ctx, pool: pool, store: store, clock: clk}
	e.docs = documents.NewService(documents.Deps{DB: pool, Store: store, Sales: gate, Logger: logger, Now: clk.Now})
	e.sales = NewService(Deps{DB: pool, Documents: e.docs, Store: store, Logger: logger, Now: clk.Now})
	e.tasks = tasks.NewService(tasks.Deps{DB: pool, Sales: gate, Documents: e.docs, Logger: logger, Now: clk.Now})
	e.customers = customers.NewService(customers.Deps{DB: pool, Logger: logger})

	e.admin = e.insertEmployee(auth.RoleSystemAdmin)
	e.manager = e.insertEmployee(auth.RoleSalesManager)
	e.advisor = e.insertEmployee(auth.RoleSalesAdvisor)
	e.other = e.insertEmployee(auth.RoleSalesAdvisor)
	return e
}

// replica runs fn on a connection where foreign keys and triggers are off, so cleanup can remove audit rows
// and skip the rules these tests are not about.
func (e *flowEnv) replica(fn func(c *pgxpool.Conn)) {
	conn, err := e.pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()
	_, _ = conn.Exec(context.Background(), `SET session_replication_role = replica`)
	fn(conn)
	_, _ = conn.Exec(context.Background(), `RESET session_replication_role`)
}

func (e *flowEnv) insertEmployee(role auth.Role) testEmployee {
	e.t.Helper()
	id := ids.New()
	emp := testEmployee{ID: id, Role: role}
	_, err := e.pool.Exec(e.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', 'Flujo', 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
		        '999000000', $3, $4::text::employee_role, $5)`,
		id, fmt.Sprintf("%08d", rand.IntN(100_000_000)), fmt.Sprintf("flow-%s@test.pe", id), string(role), auth.SystemActorID)
	if err != nil {
		e.t.Fatalf("insert employee: %v", err)
	}
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE actor_employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM lookup_logs WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM employees WHERE id = $1`, id)
		})
	})
	return emp
}

const alphabet = "ABCDEFGHJKLMNPRSTUVWXYZ"

func randFrom(chars string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = chars[rand.IntN(len(chars))]
	}
	return string(out)
}

func randDigits(n int) string { return randFrom("0123456789", n) }
func randAlpha(n int) string  { return randFrom(alphabet, n) }
func randAlnum(n int) string  { return randFrom(alphabet+"0123456789", n) }
func randVIN() string         { return randFrom("ABCDEFGHJKLMNPRSTUVWXYZ0123456789", 17) }

func (e *flowEnv) newCustomer() *customers.Customer {
	e.t.Helper()
	c, err := e.customers.Create(e.ctx, e.advisor.actor(), customers.CreateInput{
		CustomerType: customers.TypeDiplomat, DocumentType: "DNI", DocumentNumber: "00" + randDigits(6),
		FirstName: "Lucia", LastName: "Flujo" + randAlpha(8), Gender: "F", Nationality: "Peruana",
		Phone: "999111222", Email: "flow-" + uuid.NewString() + "@test.pe",
	})
	if err != nil {
		e.t.Fatalf("create customer: %v", err)
	}
	e.t.Cleanup(func() {
		e.replica(func(c2 *pgxpool.Conn) {
			_, _ = c2.Exec(context.Background(), `DELETE FROM audit_logs WHERE entity_id = $1`, c.ID)
			_, _ = c2.Exec(context.Background(), `DELETE FROM customers WHERE id = $1`, c.ID)
		})
	})
	return c
}

// newCatalog creates an active model with one version, one color and its Main image, as one transaction so the
// commit-time catalog rules see a complete model.
func (e *flowEnv) newCatalog() catalog {
	e.t.Helper()
	cat := catalog{ModelID: ids.New(), VersionID: ids.New(), ColorID: ids.New()}
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		e.t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(e.ctx) }()
	steps := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO vehicle_models (id, model_name, created_by) VALUES ($1, $2, $3)`, []any{cat.ModelID, "Hilux Flujo " + randAlpha(8), e.admin.ID}},
		{`INSERT INTO vehicle_model_categories (model_id, category) VALUES ($1, 'camioneta')`, []any{cat.ModelID}},
		{`INSERT INTO vehicle_versions (id, model_id, version_name, reference_price, created_by) VALUES ($1, $2, 'SR', 35000, $3)`, []any{cat.VersionID, cat.ModelID, e.admin.ID}},
		{`INSERT INTO vehicle_colors (id, model_id, color_name, created_by) VALUES ($1, $2, 'Blanco', $3)`, []any{cat.ColorID, cat.ModelID, e.admin.ID}},
		{`INSERT INTO catalog_images (id, vehicle_color_id, viewpoint, mime_type, size_bytes, storage_key, uploaded_by)
			VALUES ($1, $2, 'main_view', 'image/png', 1000, $3, $4)`, []any{ids.New(), cat.ColorID, "flow/" + uuid.NewString() + ".png", e.admin.ID}},
	}
	for _, s := range steps {
		if _, err := tx.Exec(e.ctx, s.sql, s.args...); err != nil {
			e.t.Fatalf("catalog fixture: %v", err)
		}
	}
	if err := tx.Commit(e.ctx); err != nil {
		e.t.Fatalf("catalog fixture commit: %v", err)
	}
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM catalog_images WHERE vehicle_color_id = $1`, cat.ColorID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_colors WHERE model_id = $1`, cat.ModelID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_versions WHERE model_id = $1`, cat.ModelID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_model_categories WHERE model_id = $1`, cat.ModelID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_models WHERE id = $1`, cat.ModelID)
		})
	})
	return cat
}

// trackSale removes everything that belongs to a sale when the test ends. With triggers off nothing cascades,
// so every table is listed.
func (e *flowEnv) trackSale(id uuid.UUID) {
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM storage_deletions WHERE storage_key LIKE 'documents/' || $1::text || '/%'`, id)
			for _, table := range []string{"documents", "sale_invoices", "sale_payments", "sale_fulfillment", "sale_due_diligence", "sale_financials", "task_instances"} {
				_, _ = c.Exec(ctx, `DELETE FROM `+table+` WHERE sale_id = $1`, id)
			}
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE sale_id = $1 OR entity_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM sale_registers WHERE id = $1`, id)
		})
	})
}

// newSale creates a sale for the customer. An advisor gets it assigned to themselves; Manager and Admin assign it
// to the default advisor.
func (e *flowEnv) newSale(actor testEmployee, cust *customers.Customer, cat catalog) *Detail {
	e.t.Helper()
	in := CreateInput{CustomerID: cust.ID, VersionID: cat.VersionID, ColorID: cat.ColorID, Temperature: "hot"}
	if actor.Role != auth.RoleSalesAdvisor {
		in.AssignedAdvisorID = &e.advisor.ID
	}
	sale, err := e.sales.Create(e.ctx, actor.actor(), in)
	if err != nil {
		e.t.Fatalf("create sale: %v", err)
	}
	e.trackSale(sale.ID)
	return sale
}

// freshSale creates a customer, a catalog entry and a sale owned by the default advisor.
func (e *flowEnv) freshSale() (*Detail, *customers.Customer, catalog) {
	e.t.Helper()
	cust, cat := e.newCustomer(), e.newCatalog()
	return e.newSale(e.advisor, cust, cat), cust, cat
}

// --- files ---

func pdfBytes() []byte { return []byte("%PDF-1.7\n% flow test " + uuid.NewString() + "\n%%EOF") }

func xlsxBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "xl/workbook.xml"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte("<x/>"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fileFor returns bytes that the slot accepts, according to the slot's allowed format.
func (e *flowEnv) fileFor(slot string) []byte {
	e.t.Helper()
	var format string
	if err := e.pool.QueryRow(e.ctx, `SELECT allowed_format::text FROM document_slots WHERE slot_code = $1`, slot).Scan(&format); err != nil {
		e.t.Fatalf("slot %s: %v", slot, err)
	}
	if format == "excel" {
		return xlsxBytes(e.t)
	}
	return pdfBytes()
}

// --- task helpers ---

func (e *flowEnv) task(actor testEmployee, saleID uuid.UUID, n int) *tasks.Detail {
	e.t.Helper()
	d, err := e.tasks.Get(e.ctx, actor.actor(), saleID, n)
	if err != nil {
		e.t.Fatalf("get task %d: %v", n, err)
	}
	return d
}

func (e *flowEnv) saveData(actor testEmployee, saleID uuid.UUID, n int, data any, reason string) (*tasks.SaveResult, error) {
	e.t.Helper()
	version := e.task(e.admin, saleID, n).Version
	raw, err := json.Marshal(data)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.tasks.SaveData(e.ctx, actor.actor(), saleID, n, tasks.SaveInput{Version: &version, Reason: reason, Data: raw})
}

func (e *flowEnv) mustSave(actor testEmployee, saleID uuid.UUID, n int, data any) *tasks.SaveResult {
	e.t.Helper()
	r, err := e.saveData(actor, saleID, n, data, "")
	if err != nil {
		e.t.Fatalf("save task %d: %v", n, err)
	}
	return r
}

func (e *flowEnv) upload(actor testEmployee, saleID uuid.UUID, n int, slot string, data []byte, reason string) (*documents.File, bool, error) {
	return e.docs.Upload(e.ctx, actor.actor(), documents.UploadInput{
		SaleID: saleID, Number: n, Slot: slot, FileName: slot + ".bin", Data: data, Reason: reason,
	})
}

func (e *flowEnv) uploadRequired(actor testEmployee, saleID uuid.UUID, n int) {
	e.t.Helper()
	for _, slot := range tasks.RequiredSlots(n, customers.TypeDiplomat) {
		if _, _, err := e.upload(actor, saleID, n, slot, e.fileFor(slot), ""); err != nil {
			e.t.Fatalf("upload %s: %v", slot, err)
		}
	}
}

func (e *flowEnv) start(actor testEmployee, saleID uuid.UUID, n int) (*tasks.Detail, error) {
	return e.tasks.Start(e.ctx, actor.actor(), saleID, n)
}

func (e *flowEnv) complete(actor testEmployee, saleID uuid.UUID, n int) (*tasks.Detail, error) {
	return e.tasks.Complete(e.ctx, actor.actor(), saleID, n)
}

// dataFor returns valid data for a task. Amounts follow one story: 30000 to Toyota, 3000 commission, 1000 of
// accessories and 1000 of release processing, a quoted price of 35000.
func (e *flowEnv) dataFor(n int) map[string]any {
	today := e.clock.Now().Format("2006-01-02")
	switch n {
	case 1:
		return map[string]any{"initialToyotaAmount": "30000", "initialCommission": "3000", "accessoryAmount": "1000", "releaseProcessingAmount": "1000"}
	case 2:
		return map[string]any{
			"signerDocumentType": "DNI", "signerDocumentNumber": "12345678", "signerFirstName": "Ana",
			"signerLastName": "Rojas", "pepStatus": "no_pep", "ddcRegime": "general",
		}
	case 3:
		return map[string]any{"vehicleSeries": "SER" + randAlnum(8), "vin": randVIN(), "finalToyotaAmount": "30000"}
	case 4:
		return map[string]any{"bank": "bcp", "paymentDate": today, "transactionNumber": "OP" + randDigits(8), "depositedAmount": "35000"}
	case 5:
		return map[string]any{"bank": "bbva", "paymentDate": today, "transactionNumber": "OP" + randDigits(8), "depositedAmount": "30000"}
	case 6:
		invoice := func(series, amount string) map[string]any {
			return map[string]any{"number": series + "-" + randDigits(6), "date": today, "amount": amount}
		}
		return map[string]any{
			"toyota": invoice("F001", "30000"), "commission": invoice("F002", "3000"),
			"accessories": invoice("F003", "1000"), "releaseProcessing": invoice("F004", "1000"),
		}
	case 8:
		return map[string]any{"damNumber": "DAM" + randDigits(8), "customsOffice": "callao_maritima", "controlChannel": "green"}
	case 9:
		return map[string]any{"personInCharge": "Juan Perez"}
	case 10:
		return map[string]any{"licensePlate": randAlnum(6), "registrationDate": today}
	case 11:
		return map[string]any{"transferDate": today}
	case 12:
		return map[string]any{"serviceOrderNumber": "OS-" + randDigits(6), "completionDate": today}
	case 13:
		return map[string]any{"deliveryDate": today}
	}
	return map[string]any{}
}

// runTask starts a task, fills it with valid data (merged with override), uploads its documents and completes it,
// all as the sale's advisor.
func (e *flowEnv) runTask(saleID uuid.UUID, n int, override map[string]any) *tasks.Detail {
	e.t.Helper()
	if _, err := e.start(e.advisor, saleID, n); err != nil {
		e.t.Fatalf("start task %d: %v", n, err)
	}
	data := e.dataFor(n)
	for k, v := range override {
		data[k] = v
	}
	e.mustSave(e.advisor, saleID, n, data)
	e.uploadRequired(e.advisor, saleID, n)
	d, err := e.complete(e.advisor, saleID, n)
	if err != nil {
		e.t.Fatalf("complete task %d: %v", n, err)
	}
	e.clock.Advance(time.Minute)
	return d
}

func (e *flowEnv) runThrough(saleID uuid.UUID, upTo int) {
	e.t.Helper()
	for n := 1; n <= upTo; n++ {
		e.runTask(saleID, n, nil)
	}
}

// --- assertions ---

func codeOf(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if got := codeOf(err); got != want {
		t.Fatalf("got error %v (code %q), want code %q", err, got, want)
	}
}

func fieldCodes(err error) map[string]string {
	out := map[string]string{}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		for _, f := range appErr.Fields {
			out[f.Field] = f.Code
		}
	}
	return out
}

func (e *flowEnv) countAudit(saleID uuid.UUID, entity, action string) int {
	e.t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_logs WHERE sale_id = $1
		AND entity_type = $2::text::audit_entity_type AND action = $3::text::audit_action`, saleID, entity, action).Scan(&n)
	if err != nil {
		e.t.Fatalf("count audit: %v", err)
	}
	return n
}

func (e *flowEnv) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("count: %v", err)
	}
	return n
}
