package catalog

import (
	"bytes"
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a migrated database (TEST_DATABASE_URL). They create their own employees and models and
// remove them afterwards.

const (
	testSecret   = "0123456789abcdef0123456789abcdef"
	goodPassword = "Correct-Horse-Battery-9"
)

type testEmployee struct {
	ID        uuid.UUID
	DocNumber string
	Role      auth.Role
}

func (e testEmployee) actor() auth.Actor { return auth.Actor{ID: e.ID, Role: e.Role} }

type testEnv struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	auth   *auth.Service
	store  *storage.Memory
	svc    *Service
	logger *slog.Logger
}

func newTestEnv(t *testing.T) *testEnv {
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
	store := storage.NewMemory("https://img.test")
	authService := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: testSecret, AppBaseURL: "http://app.test",
		PortalBaseURL: "http://portal.test", Logger: logger,
	})
	return &testEnv{
		t: t, ctx: ctx, pool: pool, auth: authService, store: store,
		svc: NewService(Deps{DB: pool, Store: store, Logger: logger}), logger: logger,
	}
}

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
		VALUES ($1, 'DNI', $2, 'Prueba', 'Catalogo', 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
				'999000000', $3, $4::text::employee_role, $5, $6, $7)`,
		id, emp.DocNumber, fmt.Sprintf("staff-%s@test.pe", id), string(role), hash, activated, auth.SystemActorID)
	if err != nil {
		e.t.Fatalf("insert employee: %v", err)
	}
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE actor_employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM access_logs WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM employee_refresh_tokens WHERE employee_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM employees WHERE id = $1`, id)
		})
	})
	return emp
}

// trackModel removes a model and everything under it when the test ends.
func (e *testEnv) trackModel(id uuid.UUID) {
	e.t.Cleanup(func() {
		e.replica(func(c *pgxpool.Conn) {
			ctx := context.Background()
			_, _ = c.Exec(ctx, `DELETE FROM sale_registers WHERE vehicle_model_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM audit_logs WHERE entity_id = $1
				OR entity_id IN (SELECT id FROM vehicle_versions WHERE model_id = $1)
				OR entity_id IN (SELECT id FROM vehicle_colors WHERE model_id = $1)
				OR entity_id IN (SELECT i.id FROM catalog_images i JOIN vehicle_colors c ON c.id = i.vehicle_color_id WHERE c.model_id = $1)`, id)
			_, _ = c.Exec(ctx, `DELETE FROM storage_deletions WHERE storage_key IN
				(SELECT i.storage_key FROM catalog_images i JOIN vehicle_colors c ON c.id = i.vehicle_color_id WHERE c.model_id = $1)
				OR storage_key LIKE 'catalog/colors/%' AND created_at > now() - interval '1 hour'`, id)
			_, _ = c.Exec(ctx, `DELETE FROM catalog_images WHERE vehicle_color_id IN (SELECT id FROM vehicle_colors WHERE model_id = $1)`, id)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_colors WHERE model_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_versions WHERE model_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_model_categories WHERE model_id = $1`, id)
			_, _ = c.Exec(ctx, `DELETE FROM vehicle_models WHERE id = $1`, id)
		})
	})
}

func pngBytes(seed int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: uint8(seed), A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func jpegBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func webpBytes() []byte {
	return append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
}

func uniqueName(prefix string) string { return fmt.Sprintf("%s %s", prefix, uuid.NewString()[:8]) }

func newModelInput() CreateModelInput {
	return CreateModelInput{
		Name: uniqueName("Hilux"), Description: "Pick-up de prueba", Categories: []string{"camioneta", "comercial"},
		Version: VersionInput{Name: "SR 4x2", ReferencePrice: Money{Raw: "35000", Set: true}},
		Color:   ColorInput{Name: "Blanco"},
	}
}

func (e *testEnv) createModel(actor auth.Actor) *Model {
	e.t.Helper()
	m, err := e.svc.CreateModel(e.ctx, actor, newModelInput(), pngBytes(1))
	if err != nil {
		e.t.Fatalf("create model: %v", err)
	}
	e.trackModel(m.ID)
	return m
}

func (e *testEnv) countAudit(entity uuid.UUID, action string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = $2::text::audit_action`, entity, action).Scan(&n); err != nil {
		e.t.Fatalf("count audit: %v", err)
	}
	return n
}

func (e *testEnv) addSale(m *Model, advisor uuid.UUID) {
	e.t.Helper()
	e.replica(func(c *pgxpool.Conn) {
		customerID := ids.New()
		_, err := c.Exec(e.ctx, `INSERT INTO customers (id, customer_type, document_type, document_number, first_name, last_name, gender,
				nationality, phone, email, created_by)
			VALUES ($1, 'diplomatico', 'CE', $2, 'Ana', 'Prueba', 'F', 'Peruana', '999000000', $3, $4)`,
			customerID, fmt.Sprintf("%09d", rand.IntN(1_000_000_000)), "ana-"+customerID.String()+"@test.pe", advisor)
		if err != nil {
			e.t.Fatalf("insert customer: %v", err)
		}
		e.t.Cleanup(func() {
			e.replica(func(c *pgxpool.Conn) {
				_, _ = c.Exec(context.Background(), `DELETE FROM sale_registers WHERE customer_id = $1`, customerID)
				_, _ = c.Exec(context.Background(), `DELETE FROM customers WHERE id = $1`, customerID)
			})
		})
		_, err = c.Exec(e.ctx, `INSERT INTO sale_registers
			(id, customer_id, customer_legal_name_snapshot, customer_doc_type_snapshot, customer_doc_number_snapshot,
			 customer_type_snapshot, vehicle_model_id, vehicle_version_id, vehicle_color_id,
			 vehicle_model_name_snapshot, vehicle_version_name_snapshot, vehicle_color_name_snapshot,
			 vehicle_reference_price_snapshot, sale_temperature, assigned_advisor_id, created_by)
			VALUES ($1, $2, 'Ana Prueba', 'CE', '000000001', 'diplomatico', $3, $4, $5, $6, $7, $8, 35000, 'hot', $9, $9)`,
			ids.New(), customerID, m.ID, m.Versions[0].ID, m.Colors[0].ID, m.Name, m.Versions[0].Name, m.Colors[0].Name, advisor)
		if err != nil {
			e.t.Fatalf("insert sale: %v", err)
		}
	})
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

func ptrTo[T any](v T) *T { return &v }

func money(raw string) Money { return Money{Raw: raw, Set: true} }
