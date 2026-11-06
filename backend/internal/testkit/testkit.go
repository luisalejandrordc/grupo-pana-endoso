// Package testkit builds the database fixtures that the integration tests of several packages share:
// employees, customers, catalog entries and documents, and the cleanup of everything a test created. It is only
// imported from tests and skips them when no test database is configured.
package testkit

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Password is the password of every employee that Employee creates with a login.
const Password = "Correct-Horse-Battery-9"

type discardMail struct{}

func (discardMail) Send(context.Context, mail.Message) error { return nil }

// Env is a connection to the test database plus the services most tests need.
type Env struct {
	T      *testing.T
	Ctx    context.Context
	Pool   *pgxpool.Pool
	Auth   *auth.Service
	Logger *slog.Logger
}

// New connects to TEST_DATABASE_URL, or skips the test when it is not set.
func New(t *testing.T) *Env {
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
	return &Env{
		T: t, Ctx: ctx, Pool: pool, Logger: logger,
		Auth: auth.NewService(auth.Deps{DB: pool, JWTSecret: "0123456789abcdef0123456789abcdef", AppBaseURL: "http://app.test", Mail: discardMail{}, Logger: logger}),
	}
}

// Replica runs fn on a connection where foreign keys and triggers are off, for fixtures and cleanup.
func (e *Env) Replica(fn func(c *pgxpool.Conn)) {
	conn, err := e.Pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()
	_, _ = conn.Exec(context.Background(), `SET session_replication_role = replica`)
	fn(conn)
	_, _ = conn.Exec(context.Background(), `RESET session_replication_role`)
}

// Employee is a test employee.
type Employee struct {
	ID        uuid.UUID
	DocNumber string
	Role      auth.Role
	Email     string
	Name      string
}

// Actor is the employee as the services see them.
func (e Employee) Actor() auth.Actor { return auth.Actor{ID: e.ID, Role: e.Role} }

// Employee inserts an active employee. With withLogin the employee can sign in with Password.
func (e *Env) Employee(role auth.Role, withLogin bool) Employee {
	e.T.Helper()
	id := ids.New()
	emp := Employee{
		ID: id, DocNumber: fmt.Sprintf("%08d", rand.IntN(100_000_000)), Role: role,
		Email: fmt.Sprintf("staff-%s@test.pe", id), Name: "Prueba " + string(role),
	}
	var hash *string
	var activated *time.Time
	if withLogin {
		h, err := auth.HashPassword(Password)
		if err != nil {
			e.T.Fatalf("hash: %v", err)
		}
		hash = &h
		now := time.Now()
		activated = &now
	}
	_, err := e.Pool.Exec(e.Ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, password_hash, activated_at, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', $3, 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
				'999000000', $4, $5::text::employee_role, $6, $7, $8)`,
		id, emp.DocNumber, string(role), emp.Email, string(role), hash, activated, auth.SystemActorID)
	if err != nil {
		e.T.Fatalf("insert employee: %v", err)
	}
	emp.Name = "Prueba " + string(role)
	e.T.Cleanup(func() {
		e.Replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			// Sales this employee created or handled, with everything under them.
			sales := `SELECT id FROM sale_registers WHERE created_by = $1 OR assigned_advisor_id = $1`
			for _, stmt := range []string{
				`DELETE FROM documents WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM task_instances WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM sale_financials WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM sale_due_diligence WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM sale_payments WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM sale_invoices WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM sale_fulfillment WHERE sale_id IN (` + sales + `)`,
				`DELETE FROM notification_logs WHERE sale_id IN (` + sales + `) OR recipient_employee_id = $1`,
				`DELETE FROM audit_logs WHERE sale_id IN (` + sales + `) OR actor_employee_id = $1`,
				`DELETE FROM sale_registers WHERE created_by = $1 OR assigned_advisor_id = $1`,
				`DELETE FROM customers WHERE created_by = $1`,
				`DELETE FROM lookup_logs WHERE employee_id = $1`,
				`DELETE FROM access_logs WHERE employee_id = $1`,
				`DELETE FROM employee_refresh_tokens WHERE employee_id = $1`,
				`DELETE FROM employees WHERE id = $1`,
			} {
				_, _ = c.Exec(ctx, stmt, id)
			}
		})
	})
	return emp
}

// Token signs the employee in and returns an access token.
func (e *Env) Token(emp Employee) string {
	e.T.Helper()
	session, err := e.Auth.Login(e.Ctx, auth.LoginRequest{DocumentType: "DNI", DocumentNumber: emp.DocNumber, Password: Password}, auth.ClientInfo{})
	if err != nil {
		e.T.Fatalf("login: %v", err)
	}
	return session.AccessToken
}

// Customer inserts an active customer (a Diplomático unless an organization is asked for) created by the
// employee.
func (e *Env) Customer(createdBy uuid.UUID, organization bool) uuid.UUID {
	e.T.Helper()
	id := ids.New()
	var err error
	if organization {
		_, err = e.Pool.Exec(e.Ctx, `INSERT INTO customers (id, customer_type, document_type, document_number, legal_name, nationality, phone, email,
				contact_first_name, contact_last_name, contact_job_title, created_by)
			VALUES ($1, 'embajada', 'RUC', $2, $3, 'Japonesa', '014440000', $4, 'Hiro', 'Tanaka', 'Agregado', $5)`,
			id, fmt.Sprintf("20%09d", rand.IntN(1_000_000_000)), "Embajada de Prueba "+id.String()[28:], "emb-"+id.String()+"@test.pe", createdBy)
	} else {
		_, err = e.Pool.Exec(e.Ctx, `INSERT INTO customers (id, customer_type, document_type, document_number, first_name, last_name, gender, nationality, phone, email, created_by)
			VALUES ($1, 'diplomatico', 'DNI', $2, 'Lucía', 'Quispe', 'F', 'Peruana', '999111222', $3, $4)`,
			id, fmt.Sprintf("%08d", rand.IntN(100_000_000)), "cli-"+id.String()+"@test.pe", createdBy)
	}
	if err != nil {
		e.T.Fatalf("insert customer: %v", err)
	}
	return id
}

// Vehicle is a selectable model, version and color.
type Vehicle struct {
	ModelID, VersionID, ColorID uuid.UUID
}

// Vehicle inserts an active model with one version, one color and its Main view image. Everything created is
// removed when the test ends.
func (e *Env) Vehicle(createdBy uuid.UUID) Vehicle {
	e.T.Helper()
	v := Vehicle{ModelID: ids.New(), VersionID: ids.New(), ColorID: ids.New()}
	tag := v.ModelID.String()[28:]
	e.Replica(func(c *pgxpool.Conn) {
		ctx := e.Ctx
		for _, step := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO vehicle_models (id, model_name, created_by) VALUES ($1, $2, $3)`, []any{v.ModelID, "Hilux " + tag, createdBy}},
			{`INSERT INTO vehicle_model_categories (model_id, category) VALUES ($1, 'camioneta')`, []any{v.ModelID}},
			{`INSERT INTO vehicle_versions (id, model_id, version_name, reference_price, created_by) VALUES ($1, $2, 'SR 4x2', 35000, $3)`, []any{v.VersionID, v.ModelID, createdBy}},
			{`INSERT INTO vehicle_colors (id, model_id, color_name, created_by) VALUES ($1, $2, 'Blanco', $3)`, []any{v.ColorID, v.ModelID, createdBy}},
			{`INSERT INTO catalog_images (id, vehicle_color_id, viewpoint, mime_type, size_bytes, storage_key, uploaded_by)
				VALUES ($1, $2, 'main_view', 'image/png', 100, $3, $4)`, []any{ids.New(), v.ColorID, "test/" + tag + ".png", createdBy}},
		} {
			if _, err := c.Exec(ctx, step.sql, step.args...); err != nil {
				e.T.Fatalf("insert catalog: %v", err)
			}
		}
	})
	e.T.Cleanup(func() {
		e.Replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM catalog_images WHERE vehicle_color_id = $1`, v.ColorID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_colors WHERE id = $1`, v.ColorID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_versions WHERE id = $1`, v.VersionID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_model_categories WHERE model_id = $1`, v.ModelID)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_models WHERE id = $1`, v.ModelID)
		})
	})
	return v
}

// Document records that a file was uploaded to a slot of a task, as the documents module will do.
func (e *Env) Document(saleID, taskID, uploader uuid.UUID, slotCode string) {
	e.T.Helper()
	_, err := e.Pool.Exec(e.Ctx, `INSERT INTO documents (id, sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes, storage_key, checksum_sha256, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, 'application/pdf', 1000, $6, $7, $8)`,
		ids.New(), saleID, taskID, slotCode, slotCode+".pdf", "test/docs/"+ids.New().String()+".pdf", fmt.Sprintf("%064x", rand.Uint64()), uploader)
	if err != nil {
		e.T.Fatalf("insert document: %v", err)
	}
}

// FillRequiredDocuments records a document for every slot of a task, which is what completing it requires.
func (e *Env) FillRequiredDocuments(saleID uuid.UUID, taskNumber int, uploader uuid.UUID) {
	e.T.Helper()
	var taskID uuid.UUID
	if err := e.Pool.QueryRow(e.Ctx, `SELECT id FROM task_instances WHERE sale_id = $1 AND task_number = $2`, saleID, taskNumber).Scan(&taskID); err != nil {
		e.T.Fatalf("find task: %v", err)
	}
	rows, err := e.Pool.Query(e.Ctx, `SELECT slot_code FROM document_slots WHERE task_number = $1`, taskNumber)
	if err != nil {
		e.T.Fatalf("slots: %v", err)
	}
	var slots []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.T.Fatal(err)
		}
		slots = append(slots, s)
	}
	rows.Close()
	for _, s := range slots {
		var exists bool
		_ = e.Pool.QueryRow(e.Ctx, `SELECT EXISTS (SELECT 1 FROM documents WHERE task_instance_id = $1 AND slot_code = $2)`, taskID, s).Scan(&exists)
		if !exists {
			e.Document(saleID, taskID, uploader, s)
		}
	}
}
