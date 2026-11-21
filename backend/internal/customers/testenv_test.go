package customers

import (
	"context"
	"endosoflow/internal/adapters/codart"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a migrated database: run `make db-verify` once, then `make test`. They create their
// own employees, customers and sales and remove them afterwards.

const (
	testSecret   = "0123456789abcdef0123456789abcdef"
	goodPassword = "Correct-Horse-Battery-9"
)

// fakeIdentity stands in for Codart and counts the calls that reach it.
type fakeIdentity struct {
	mu    sync.Mutex
	calls int
	dni   func(string) (codart.Person, error)
	ruc   func(string) (string, error)
}

func (f *fakeIdentity) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeIdentity) LookupDNI(_ context.Context, dni string) (codart.Person, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.dni != nil {
		return f.dni(dni)
	}
	return codart.Person{FirstName: "LUCIA", LastName: "QUISPE MAMANI"}, nil
}

func (f *fakeIdentity) LookupRUC(_ context.Context, ruc string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.ruc != nil {
		return f.ruc(ruc)
	}
	return "EMBAJADA DE PRUEBA", nil
}

type testEnv struct {
	queue    *activationRecorder
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	auth     *auth.Service
	identity *fakeIdentity
	svc      *Service
	logger   *slog.Logger
}

type testEmployee struct {
	ID        uuid.UUID
	DocNumber string
	Role      auth.Role
}

func (e testEmployee) actor() auth.Actor { return auth.Actor{ID: e.ID, Role: e.Role} }

func newTestEnv(t *testing.T, tweak ...func(*Deps)) *testEnv {
	t.Helper()
	dsn := testDSN(t)
	ctx := context.Background()
	pool, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	identity := &fakeIdentity{}
	// The real auth service validates and queues, behind a recorder that stands in for the notifications writer
	// (the notifications package depends on this one, so the real writer cannot be used here).
	queue := &activationRecorder{}
	authService := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: testSecret, AppBaseURL: "http://app.test", PortalBaseURL: "http://portal.test", Notifier: queue, Logger: logger,
	})
	deps := Deps{DB: pool, Activation: authService, Identity: identity, Logger: logger}
	for _, f := range tweak {
		f(&deps)
	}
	return &testEnv{t: t, ctx: ctx, pool: pool, auth: authService, identity: identity, svc: NewService(deps), logger: logger, queue: queue}
}

// replica runs fn on a connection where foreign keys and triggers are off, so fixtures and cleanup can
// skip the rules these tests are not about.
func (e *testEnv) replica(fn func(c *pgxpool.Conn)) {
	conn, err := e.pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()
	_, _ = conn.Exec(context.Background(), `SET session_replication_role = replica`)
	fn(conn)
	_, _ = conn.Exec(context.Background(), `RESET session_replication_role`)
}

func (e *testEnv) insertEmployee(role auth.Role, password string) testEmployee {
	e.t.Helper()
	id := ids.New()
	emp := testEmployee{ID: id, DocNumber: fmt.Sprintf("%08d", rand.IntN(100_000_000)), Role: role}
	var hash *string
	var activated *time.Time
	if password != "" {
		h, err := auth.HashPassword(password)
		if err != nil {
			e.t.Fatalf("hash: %v", err)
		}
		hash = &h
		now := time.Now()
		activated = &now
	}
	_, err := e.pool.Exec(e.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, password_hash, activated_at, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', 'Cliente', 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
				'999000000', $3, $4::text::employee_role, $5, $6, $7)`,
		id, emp.DocNumber, fmt.Sprintf("staff-%s@test.pe", id), string(role), hash, activated, auth.SystemActorID)
	if err != nil {
		e.t.Fatalf("insert employee: %v", err)
	}
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM lookup_logs WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE actor_employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM access_logs WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM employee_refresh_tokens WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM employees WHERE id = $1`, id)
		})
	})
	return emp
}

// trackCustomer removes the customer, its sales and its audit rows when the test ends.
func (e *testEnv) trackCustomer(id uuid.UUID) {
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM account_tokens WHERE customer_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM notification_logs WHERE recipient_customer_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM sale_registers WHERE customer_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE entity_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM customers WHERE id = $1`, id)
		})
	})
}

func (e *testEnv) createCustomer(actor auth.Actor, in CreateInput) *Customer {
	e.t.Helper()
	c, err := e.svc.Create(e.ctx, actor, in)
	if err != nil {
		e.t.Fatalf("create customer: %v", err)
	}
	e.trackCustomer(c.ID)
	return c
}

// addSale inserts a minimal sale for the customer, assigned to the advisor.
func (e *testEnv) addSale(customer *Customer, advisor uuid.UUID) uuid.UUID {
	e.t.Helper()
	id := ids.New()
	var execErr error
	e.replica(func(c *pgxpool.Conn) {
		_, execErr = c.Exec(e.ctx, `INSERT INTO sale_registers
			(id, customer_id, customer_legal_name_snapshot, customer_doc_type_snapshot, customer_doc_number_snapshot,
			 customer_type_snapshot, vehicle_model_id, vehicle_version_id, vehicle_color_id,
			 vehicle_model_name_snapshot, vehicle_version_name_snapshot, vehicle_color_name_snapshot,
			 vehicle_reference_price_snapshot, sale_temperature, assigned_advisor_id, created_by)
			VALUES ($1, $2, $3, $4::text::customer_document_type, $5, $6::text::customer_type,
					$7, $8, $9, 'Hilux', 'SR', 'Blanco', 35000, 'hot', $10, $10)`,
			id, customer.ID, customer.DisplayName, customer.DocumentType, customer.DocumentNumber,
			customer.CustomerType, ids.New(), ids.New(), ids.New(), advisor)
	})
	if execErr != nil {
		e.t.Fatalf("insert sale: %v", execErr)
	}
	return id
}

func (e *testEnv) reassign(saleID, advisor uuid.UUID) {
	e.t.Helper()
	e.replica(func(c *pgxpool.Conn) {
		if _, err := c.Exec(e.ctx, `UPDATE sale_registers SET assigned_advisor_id = $2 WHERE id = $1`, saleID, advisor); err != nil {
			e.t.Fatalf("reassign: %v", err)
		}
	})
}

func (e *testEnv) countAudit(entity uuid.UUID, action string) int {
	e.t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_logs
		WHERE entity_id = $1 AND action = $2::text::audit_action`, entity, action).Scan(&n)
	if err != nil {
		e.t.Fatalf("count audit: %v", err)
	}
	return n
}

func (e *testEnv) countLookupLogs(employee uuid.UUID, kind, outcome string) int {
	e.t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM lookup_logs
		WHERE employee_id = $1 AND lookup_kind = $2::text::lookup_kind AND outcome = $3::text::lookup_outcome`,
		employee, kind, outcome).Scan(&n)
	if err != nil {
		e.t.Fatalf("count lookup logs: %v", err)
	}
	return n
}

func randomDigits(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte('0' + rand.IntN(10))
	}
	return string(out)
}

func person() CreateInput {
	return CreateInput{
		CustomerType: TypeDiplomat, DocumentType: "DNI", DocumentNumber: "00" + randomDigits(6),
		FirstName: "Lucía", LastName: "Quispe", Gender: "F", Nationality: "Peruana",
		Phone: "999111222", Email: "Lucia-" + uuid.NewString() + "@Test.pe",
	}
}

func organization() CreateInput {
	return CreateInput{
		CustomerType: TypeEmbassy, DocumentType: "RUC", DocumentNumber: "20" + randomDigits(9),
		LegalName: "Embajada de Prueba " + randomDigits(4), Nationality: "Japonesa", Phone: "014440000",
		Email: "embajada-" + uuid.NewString() + "@test.pe", ContactFirstName: "Hiro",
		ContactLastName: "Tanaka", ContactJobTitle: "Agregado administrativo",
	}
}

func code(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func wantCode(t *testing.T, err error, want string) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("got error %v (code %q), want code %q", err, got, want)
	}
}

func fieldNames(err error) map[string]bool {
	names := map[string]bool{}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		for _, f := range appErr.Fields {
			names[f.Field] = true
		}
	}
	return names
}

// activationRecorder records the activation emails the auth service queues and writes the real queue row.
type activationRecorder struct {
	mu    sync.Mutex
	calls []string // customer emails
}

func (a *activationRecorder) CustomerActivation(ctx context.Context, tx pgx.Tx, id uuid.UUID, email string) error {
	a.mu.Lock()
	a.calls = append(a.calls, email)
	a.mu.Unlock()
	_, err := tx.Exec(ctx, `INSERT INTO notification_logs (id, notification_type, recipient_customer_id, recipient_email, dedupe_key)
		VALUES ($1, 'customer_activation', $2, $3, $4)`, ids.New(), id, email, "customer_activation:"+ids.NewString())
	return err
}

func (a *activationRecorder) EmployeeActivation(context.Context, pgx.Tx, uuid.UUID, string) error {
	return errors.New("not used by customer tests")
}

func (a *activationRecorder) EmployeeActivated(context.Context, pgx.Tx, uuid.UUID, string) error {
	return errors.New("not used by customer tests")
}

func (a *activationRecorder) CustomerActivated(context.Context, pgx.Tx, uuid.UUID, string) error {
	return nil // the confirmation is not what these tests are about
}

func (a *activationRecorder) PasswordReset(context.Context, pgx.Tx, uuid.UUID, string) error {
	return errors.New("not used by customer tests")
}

func (a *activationRecorder) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}
