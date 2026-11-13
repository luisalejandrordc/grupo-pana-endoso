package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/sales"
	"endosoflow/internal/staff"
	"endosoflow/internal/tasks"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The role matrix: who may call each route. It is written from the Architecture & API document (section 7),
// not read from the handlers, so a handler that drifts from it fails here. Rules that depend on the sale or
// the stage (an advisor's scope, who can cancel at which task) are covered by the flow tests.
var (
	anyStaff   = []auth.Role{auth.RoleSystemAdmin, auth.RoleSalesManager, auth.RoleSalesAdvisor}
	management = []auth.Role{auth.RoleSystemAdmin, auth.RoleSalesManager}
	adminOnly  = []auth.Role{auth.RoleSystemAdmin}
)

type rule struct {
	method, path string
	roles        []auth.Role
}

var matrix = []rule{
	// Own account
	{"GET", "/me", anyStaff},
	{"PUT", "/me/password", anyStaff},

	// Staff
	{"GET", "/employees", adminOnly},
	{"POST", "/employees", adminOnly},
	{"GET", "/employees/{id}", adminOnly},
	{"PATCH", "/employees/{id}", adminOnly},
	{"DELETE", "/employees/{id}", adminOnly},
	{"POST", "/employees/{id}/deactivate", adminOnly},
	{"POST", "/employees/{id}/reactivate", adminOnly},
	{"POST", "/employees/{id}/resend-activation", adminOnly},
	{"GET", "/advisors", management},

	// Customers
	{"GET", "/customers", anyStaff},
	{"POST", "/customers", anyStaff},
	{"GET", "/customers/lookup", anyStaff},
	{"GET", "/customers/{id}", anyStaff},
	{"PATCH", "/customers/{id}", anyStaff},
	{"DELETE", "/customers/{id}", adminOnly},
	{"POST", "/customers/{id}/deactivate", management},
	{"POST", "/customers/{id}/reactivate", management},
	{"GET", "/lookups/identity", anyStaff},
	{"GET", "/lookups/identity/quota", adminOnly},

	// Catalog
	{"GET", "/vehicle-models", anyStaff},
	{"POST", "/vehicle-models", management},
	{"GET", "/vehicle-models/{id}", anyStaff},
	{"PATCH", "/vehicle-models/{id}", management},
	{"DELETE", "/vehicle-models/{id}", adminOnly},
	{"POST", "/vehicle-models/{id}/archive", management},
	{"POST", "/vehicle-models/{id}/unarchive", management},
	{"POST", "/vehicle-models/{id}/versions", management},
	{"POST", "/vehicle-models/{id}/colors", management},
	{"PATCH", "/vehicle-versions/{id}", management},
	{"DELETE", "/vehicle-versions/{id}", adminOnly},
	{"POST", "/vehicle-versions/{id}/archive", management},
	{"POST", "/vehicle-versions/{id}/unarchive", management},
	{"PATCH", "/vehicle-colors/{id}", management},
	{"DELETE", "/vehicle-colors/{id}", adminOnly},
	{"POST", "/vehicle-colors/{id}/archive", management},
	{"POST", "/vehicle-colors/{id}/unarchive", management},
	{"PUT", "/vehicle-colors/{id}/images/{viewpoint}", management},
	{"DELETE", "/vehicle-colors/{id}/images/{viewpoint}", management},
	{"GET", "/catalog/selectable", anyStaff},

	// Sales
	{"GET", "/sales", anyStaff},
	{"POST", "/sales", anyStaff},
	{"GET", "/sales/{id}", anyStaff},
	{"PATCH", "/sales/{id}", anyStaff},
	{"DELETE", "/sales/{id}", adminOnly},
	{"GET", "/sales/{id}/history", anyStaff},
	{"POST", "/sales/{id}/reassign", management},
	{"POST", "/sales/{id}/cancel", anyStaff}, // the stage rules decide inside the service
	{"POST", "/sales/{id}/reopen", management},

	// Tasks
	{"GET", "/sales/{id}/tasks", anyStaff},
	{"GET", "/sales/{id}/tasks/{n}", anyStaff},
	{"PATCH", "/sales/{id}/tasks/{n}/data", anyStaff},
	{"PATCH", "/sales/{id}/tasks/{n}/notes", anyStaff},
	{"POST", "/sales/{id}/tasks/{n}/start", anyStaff},
	{"POST", "/sales/{id}/tasks/{n}/complete", anyStaff},
	{"POST", "/sales/{id}/tasks/{n}/restart", management},
	{"POST", "/sales/{id}/tasks/{n}/reopen", management},

	// Documents
	{"GET", "/sales/{id}/documents", anyStaff},
	{"PUT", "/sales/{id}/tasks/{n}/documents/{slot}", anyStaff},
	{"DELETE", "/sales/{id}/tasks/{n}/documents/{slot}", anyStaff},
	{"GET", "/documents/{id}/download-url", anyStaff},
}

// Routes that do not need a bearer token. Their own modules test them.
var publicRoutes = map[string]bool{
	"GET /health":                true,
	"POST /auth/employee/login":  true,
	"POST /auth/refresh":         true,
	"POST /auth/logout":          true,
	"POST /auth/activate":        true,
	"POST /auth/forgot-password": true,
	"POST /auth/reset-password":  true,
}

func routeKey(method, path string) string { return method + " " + path }

// A new route in the contract has to be given a role rule here, or declared public.
func TestEveryRouteInTheContractHasARoleRule(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load the contract: %v", err)
	}
	inContract := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			inContract[routeKey(strings.ToUpper(method), path)] = true
		}
	}
	ruled := map[string]bool{}
	for _, r := range matrix {
		key := routeKey(r.method, r.path)
		if ruled[key] {
			t.Errorf("%s has two rules", key)
		}
		ruled[key] = true
		if !inContract[key] {
			t.Errorf("%s has a role rule but is not in the contract", key)
		}
	}
	var missing []string
	for key := range inContract {
		if !ruled[key] && !publicRoutes[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		t.Errorf("%s is in the contract but has no role rule: add it to the matrix in roles_test.go (or to publicRoutes)", key)
	}
}

// --- the real stack ---

const roleTestSecret = "role-matrix-test-secret-0123456789-abcdefghij"

type roleStack struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	router http.Handler
	secret []byte
	tokens map[auth.Role]string
	ids    map[auth.Role]uuid.UUID
}

func newRoleStack(t *testing.T) *roleStack {
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
	authService := auth.NewService(auth.Deps{DB: pool, JWTSecret: roleTestSecret, AppBaseURL: "http://app.test", Mail: mail.NewLogSender(logger), Logger: logger})
	authHandler := auth.NewHandler(authService, logger, auth.HandlerConfig{AppBaseURL: "http://app.test"})
	staffHandler := staff.NewHandler(staff.NewService(staff.Deps{DB: pool, Activation: authService, Access: authService, Logger: logger}), authHandler, logger)
	customersHandler := customers.NewHandler(customers.NewService(customers.Deps{DB: pool, Logger: logger}), authHandler, logger)
	store := storage.NewMemory("https://files.test")
	catalogHandler := catalog.NewHandler(catalog.NewService(catalog.Deps{DB: pool, Store: store, Logger: logger}), authHandler, logger)
	gate := sales.Gate{}
	documentsService := documents.NewService(documents.Deps{DB: pool, Store: store, Sales: gate, Logger: logger})
	salesHandler := sales.NewHandler(sales.NewService(sales.Deps{DB: pool, Documents: documentsService, Store: store, Logger: logger}), authHandler, logger)
	tasksHandler := tasks.NewHandler(tasks.NewService(tasks.Deps{DB: pool, Sales: gate, Documents: documentsService, Logger: logger}), authHandler, logger)
	documentsHandler := documents.NewHandler(documentsService, authHandler, logger)

	s := &roleStack{
		t: t, ctx: ctx, pool: pool, secret: []byte(roleTestSecret),
		tokens: map[auth.Role]string{}, ids: map[auth.Role]uuid.UUID{},
		router: newRouter(logger, pool, authHandler.Mount, staffHandler.Mount, customersHandler.Mount, catalogHandler.Mount,
			salesHandler.Mount, tasksHandler.Mount, documentsHandler.Mount),
	}
	for _, role := range anyStaff {
		id := s.insertEmployee(role)
		s.ids[role] = id
		s.tokens[role] = s.tokenFor(id, role)
	}
	return s
}

func (s *roleStack) insertEmployee(role auth.Role) uuid.UUID {
	s.t.Helper()
	id := ids.New()
	_, err := s.pool.Exec(s.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', 'Roles', 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
		        '999000000', $3, $4::text::employee_role, $5)`,
		id, fmt.Sprintf("%08d", rand.IntN(100_000_000)), fmt.Sprintf("roles-%s@test.pe", id), string(role), auth.SystemActorID)
	if err != nil {
		s.t.Fatalf("insert employee: %v", err)
	}
	s.t.Cleanup(func() {
		conn, err := s.pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(), `SET session_replication_role = replica`)
		_, _ = conn.Exec(context.Background(), `DELETE FROM audit_logs WHERE actor_employee_id = $1`, id)
		_, _ = conn.Exec(context.Background(), `DELETE FROM employees WHERE id = $1`, id)
		_, _ = conn.Exec(context.Background(), `RESET session_replication_role`)
	})
	return id
}

func (s *roleStack) tokenFor(id uuid.UUID, role auth.Role) string {
	s.t.Helper()
	token, _, err := auth.IssueAccessToken(s.secret, id, role, time.Now())
	if err != nil {
		s.t.Fatal(err)
	}
	return token
}

// call sends a request with the given bearer token (none when empty) and returns the status and the body. A
// JSON body is sent where the route takes one, so a route that is allowed answers with a validation error or a
// not-found, never with a missing-body surprise.
func (s *roleStack) call(method, path, token string) (int, string) {
	s.t.Helper()
	path = strings.NewReplacer(
		"{id}", "0198f000-0000-7000-8000-000000000001", "{n}", "1", "{slot}", "quotation_pdf", "{viewpoint}", "main_view",
	).Replace(path)
	var body io.Reader
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
		body = strings.NewReader(`{}`)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func errorCode(body string) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &envelope)
	return envelope.Error.Code
}

func hasRole(roles []auth.Role, role auth.Role) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func TestRoleMatrixOnTheRealStack(t *testing.T) {
	s := newRoleStack(t)
	for _, r := range matrix {
		name := routeKey(r.method, r.path)
		t.Run(name, func(t *testing.T) {
			// Nobody gets in without a valid token.
			if status, body := s.call(r.method, r.path, ""); status != http.StatusUnauthorized {
				t.Errorf("no token: got %d (%s), want 401", status, errorCode(body))
			}
			if status, body := s.call(r.method, r.path, "not-a-token"); status != http.StatusUnauthorized {
				t.Errorf("garbage token: got %d (%s), want 401", status, errorCode(body))
			}
			for _, role := range anyStaff {
				status, body := s.call(r.method, r.path, s.tokens[role])
				switch {
				case hasRole(r.roles, role):
					if status == http.StatusUnauthorized || status == http.StatusForbidden {
						t.Errorf("%s must be allowed, got %d (%s)", role, status, errorCode(body))
					}
					if status >= 500 {
						t.Errorf("%s: harmless input must not cause a server error, got %d (%s)", role, status, errorCode(body))
					}
				default:
					if status != http.StatusForbidden || errorCode(body) != "FORBIDDEN" {
						t.Errorf("%s must be refused with 403 FORBIDDEN, got %d (%s)", role, status, errorCode(body))
					}
				}
			}
		})
	}
}

func TestTheHealthRouteNeedsNoToken(t *testing.T) {
	s := newRoleStack(t)
	if status, _ := s.call("GET", "/health", ""); status != http.StatusOK {
		t.Fatalf("health: %d", status)
	}
}

// The token only says who is asking. The role, and whether the account is still active, come from the database
// on every request.
func TestTokensAreCheckedAgainstTheDatabase(t *testing.T) {
	s := newRoleStack(t)
	advisor := s.ids[auth.RoleSalesAdvisor]

	t.Run("a token cannot claim a role the account does not have", func(t *testing.T) {
		forged := s.tokenFor(advisor, auth.RoleSystemAdmin)
		if status, _ := s.call("GET", "/employees", forged); status != http.StatusForbidden {
			t.Errorf("an advisor's token claiming admin got %d, want 403", status)
		}
	})
	t.Run("a role change takes effect immediately", func(t *testing.T) {
		other := s.insertEmployee(auth.RoleSalesAdvisor)
		token := s.tokenFor(other, auth.RoleSalesAdvisor)
		if status, _ := s.call("GET", "/advisors", token); status != http.StatusForbidden {
			t.Fatalf("an advisor must not read the advisor list, got %d", status)
		}
		if _, err := s.pool.Exec(s.ctx, `UPDATE employees SET role = 'sales_manager' WHERE id = $1`, other); err != nil {
			t.Fatal(err)
		}
		if status, _ := s.call("GET", "/advisors", token); status != http.StatusOK {
			t.Errorf("after the promotion the same token must be allowed, got %d", status)
		}
	})
	t.Run("a deactivated account is locked out at once", func(t *testing.T) {
		gone := s.insertEmployee(auth.RoleSalesAdvisor)
		token := s.tokenFor(gone, auth.RoleSalesAdvisor)
		if status, _ := s.call("GET", "/me", token); status != http.StatusOK {
			t.Fatalf("an active account must be allowed, got %d", status)
		}
		if _, err := s.pool.Exec(s.ctx, `UPDATE employees SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`,
			gone, s.ids[auth.RoleSystemAdmin]); err != nil {
			t.Fatal(err)
		}
		if status, _ := s.call("GET", "/me", token); status != http.StatusUnauthorized {
			t.Errorf("a deactivated account got %d, want 401", status)
		}
	})
	t.Run("an unknown account", func(t *testing.T) {
		token := s.tokenFor(uuid.New(), auth.RoleSystemAdmin)
		if status, _ := s.call("GET", "/me", token); status != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", status)
		}
	})
}

func TestTamperedExpiredAndForeignTokensAreRefused(t *testing.T) {
	s := newRoleStack(t)
	id := s.ids[auth.RoleSystemAdmin]
	good := s.tokens[auth.RoleSystemAdmin]
	if status, _ := s.call("GET", "/me", good); status != http.StatusOK {
		t.Fatalf("the control token must work, got %d", status)
	}

	parts := strings.Split(good, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	expired, _, _ := auth.IssueAccessToken(s.secret, id, auth.RoleSystemAdmin, time.Now().Add(-24*time.Hour))
	foreign, _, _ := auth.IssueAccessToken([]byte("another-secret-0123456789-abcdefghijklmnop"), id, auth.RoleSystemAdmin, time.Now())

	cases := map[string]string{
		"a changed signature":    parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-3] + "AAA",
		"a changed payload":      parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x","role":"system_admin"}`)) + "." + parts[2],
		"the none algorithm":     header + "." + parts[1] + ".",
		"an expired token":       expired,
		"a token of another key": foreign,
		"an empty bearer":        "",
		"two segments only":      parts[0] + "." + parts[1],
	}
	for name, token := range cases {
		if status, _ := s.call("GET", "/me", token); status != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, status)
		}
	}
	// The scheme must be Bearer.
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Basic "+good)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a non-Bearer scheme got %d, want 401", rec.Code)
	}
}
