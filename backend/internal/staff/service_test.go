package staff

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need a migrated database: run `make db-verify` once, then `make test`.
// They create their own employees and remove them afterwards.

const (
	testSecret   = "0123456789abcdef0123456789abcdef"
	goodPassword = "Correct-Horse-Battery-9"
)

type captureMail struct {
	mu       sync.Mutex
	messages []mail.Message
}

func (c *captureMail) Send(_ context.Context, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, m)
	return nil
}

func (c *captureMail) to(address string) []mail.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []mail.Message
	for _, m := range c.messages {
		if m.To == address {
			out = append(out, m)
		}
	}
	return out
}

type testEnv struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	auth    *auth.Service
	staff   *Service
	mail    *captureMail
	avatars *storage.Memory
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := testDSN(t)
	ctx := context.Background()
	pool, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	captured := &captureMail{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: testSecret, AppBaseURL: "http://app.test", Mail: captured, Logger: logger,
	})
	avatars := storage.NewMemory("")
	staffService := NewService(Deps{DB: pool, Activation: authService, Access: authService, Avatars: avatars, Logger: logger})
	return &testEnv{ctx: ctx, pool: pool, auth: authService, staff: staffService, mail: captured, avatars: avatars}
}

func actorOf(id uuid.UUID, role auth.Role) auth.Actor { return auth.Actor{ID: id, Role: role} }

type testEmployee struct {
	ID        uuid.UUID
	DocNumber string
	Email     string
	LastName  string
}

// cleanup removes an employee for good. Replica mode switches off the last-Admin trigger and foreign
// keys for this one connection, so test accounts can always be removed.
func (e *testEnv) cleanup(id uuid.UUID) {
	ctx := context.Background()
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	_, _ = conn.Exec(ctx, `SET session_replication_role = replica`)
	_, _ = conn.Exec(ctx, `DELETE FROM employee_refresh_tokens WHERE employee_id = $1`, id)
	_, _ = conn.Exec(ctx, `DELETE FROM account_tokens WHERE employee_id = $1`, id)
	_, _ = conn.Exec(ctx, `DELETE FROM employees WHERE id = $1`, id)
	_, _ = conn.Exec(ctx, `RESET session_replication_role`)
}

func (e *testEnv) insertEmployee(t *testing.T, role, password, lastName string) testEmployee {
	t.Helper()
	id := ids.New()
	emp := testEmployee{
		ID: id, DocNumber: fmt.Sprintf("%08d", rand.IntN(100_000_000)),
		Email: fmt.Sprintf("staff-%s@test.pe", id), LastName: lastName,
	}
	var hash *string
	var activated *time.Time
	if password != "" {
		h, err := auth.HashPassword(password)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		hash = &h
		now := time.Now()
		activated = &now
	}
	_, err := e.pool.Exec(e.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, password_hash, activated_at, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', $3, 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
				'999000000', $4, $5::text::employee_role, $6, $7, $8)`,
		id, emp.DocNumber, lastName, emp.Email, role, hash, activated, auth.SystemActorID)
	if err != nil {
		t.Fatalf("insert employee: %v", err)
	}
	t.Cleanup(func() { e.cleanup(id) })
	return emp
}

func validInput() CreateInput {
	id := uuid.NewString()
	return CreateInput{
		DocumentType: "DNI", DocumentNumber: fmt.Sprintf("%08d", rand.IntN(100_000_000)),
		FirstName: "Lucía", LastName: "Quispe", Nationality: "Peruana", Gender: "F",
		Birthdate: "1992-05-14", HireDate: "2024-02-01", BusinessPhone: "999111222",
		BusinessEmail: "new-" + id + "@test.pe", Role: "sales_advisor",
	}
}

// createViaService creates an employee through the service, as an Admin would.
func (e *testEnv) createViaService(t *testing.T, admin auth.Actor) (*CreateResult, CreateInput) {
	t.Helper()
	in := validInput()
	result, err := e.staff.Create(e.ctx, admin, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { e.cleanup(result.Employee.ID) })
	return result, in
}

func (e *testEnv) countAudit(t *testing.T, entity uuid.UUID, action string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_logs
		WHERE entity_id = $1 AND action = $2::text::audit_action`, entity, action).Scan(&n)
	if err != nil {
		t.Fatalf("count audit: %v", err)
	}
	return n
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

func fieldNames(err error) []string {
	var appErr *apperr.Error
	var names []string
	if errors.As(err, &appErr) {
		for _, f := range appErr.Fields {
			names = append(names, f.Field)
		}
	}
	return names
}

var tokenInText = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// ---- create ------------------------------------------------------------------------------------

func TestCreateIssuesAnActivationLinkAndIsAudited(t *testing.T) {
	e := newTestEnv(t)
	admin := actorOf(e.insertEmployee(t, "system_admin", goodPassword, "Admin").ID, auth.RoleSystemAdmin)

	result, in := e.createViaService(t, admin)
	if !result.ActivationLinkIssued || result.Employee.ActivatedAt != nil || result.Employee.Version != 1 ||
		!result.Employee.IsActive || result.Employee.Role != "sales_advisor" {
		t.Fatalf("unexpected result: %+v", result)
	}
	e.auth.WaitForBackgroundWork()
	if sent := e.mail.to(in.BusinessEmail); len(sent) != 1 || !strings.Contains(sent[0].Body, "/activar-cuenta?token=") {
		t.Fatalf("expected one activation email, got %+v", sent)
	}
	if n := e.countAudit(t, result.Employee.ID, "CREATED"); n != 1 {
		t.Fatalf("expected 1 CREATED audit row, got %d", n)
	}

	_, err := e.staff.Create(e.ctx, admin, in)
	if c := code(err); c != "EMPLOYEE_ALREADY_EXISTS" && c != "EMPLOYEE_EMAIL_ALREADY_USED" {
		t.Fatalf("a duplicate must be refused, got %v", err)
	}
}

func TestCreateRejectsInvalidDataAndListsEveryProblem(t *testing.T) {
	e := newTestEnv(t)
	admin := actorOf(uuid.New(), auth.RoleSystemAdmin) // validation fails before the database is used

	in := validInput()
	in.DocumentNumber = "123"
	in.FirstName = "  "
	in.Role = "boss"
	in.BusinessEmail = "not-an-email"
	in.Birthdate = "1992/05/14"
	_, err := e.staff.Create(e.ctx, admin, in)
	wantCode(t, err, "VALIDATION_FAILED")

	got := strings.Join(fieldNames(err), ",")
	for _, field := range []string{"documentNumber", "firstName", "role", "businessEmail", "birthdate"} {
		if !strings.Contains(got, field) {
			t.Errorf("the problem list %q must mention %s", got, field)
		}
	}
}

// ---- update ------------------------------------------------------------------------------------

func TestUpdateChangesFieldsChecksTheVersionAndAuditsRoleChanges(t *testing.T) {
	e := newTestEnv(t)
	admin := actorOf(e.insertEmployee(t, "system_admin", goodPassword, "Admin").ID, auth.RoleSystemAdmin)
	created, _ := e.createViaService(t, admin)
	id := created.Employee.ID

	firstName := "Renombrada"
	version := created.Employee.Version
	updated, err := e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, FirstName: &firstName})
	if err != nil || updated.FirstName != firstName || updated.Version != version+1 {
		t.Fatalf("unexpected update result: %+v, %v", updated, err)
	}
	if n := e.countAudit(t, id, "UPDATED"); n != 1 {
		t.Fatalf("expected 1 UPDATED audit row, got %d", n)
	}

	_, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, FirstName: &firstName}) // stale version
	wantCode(t, err, "VERSION_CONFLICT")
	_, err = e.staff.Update(e.ctx, admin, id, UpdateInput{FirstName: &firstName}) // no version
	wantCode(t, err, "VERSION_REQUIRED")

	role := "sales_manager"
	version = updated.Version
	updated, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, Role: &role})
	if err != nil || updated.Role != role {
		t.Fatalf("role change: %+v, %v", updated, err)
	}
	if n := e.countAudit(t, id, "ROLE_CHANGED"); n != 1 {
		t.Fatalf("expected 1 ROLE_CHANGED audit row, got %d", n)
	}

	empty := ""
	version = updated.Version
	_, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, FirstName: &empty})
	wantCode(t, err, "VALIDATION_FAILED")

	other := e.insertEmployee(t, "sales_advisor", "", "Otro")
	_, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, DocumentNumber: &other.DocNumber})
	wantCode(t, err, "EMPLOYEE_ALREADY_EXISTS")

	// Personal contact data can be set and cleared; an empty string clears it.
	phone := "999888777"
	updated, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, PersonalPhone: &phone})
	if err != nil || updated.PersonalPhone == nil || *updated.PersonalPhone != phone {
		t.Fatalf("set personal phone: %+v, %v", updated, err)
	}
	version = updated.Version
	updated, err = e.staff.Update(e.ctx, admin, id, UpdateInput{Version: &version, PersonalPhone: &empty})
	if err != nil || updated.PersonalPhone != nil {
		t.Fatalf("clear personal phone: %+v, %v", updated, err)
	}
}

// ---- deactivate and reactivate -----------------------------------------------------------------

func TestDeactivateEndsSessionsAndReactivateRestoresAccess(t *testing.T) {
	e := newTestEnv(t)
	admin := e.insertEmployee(t, "system_admin", goodPassword, "Admin")
	actor := actorOf(admin.ID, auth.RoleSystemAdmin)
	target := e.insertEmployee(t, "sales_manager", goodPassword, "Objetivo")

	login := func() (*auth.Session, error) {
		return e.auth.Login(e.ctx, auth.LoginRequest{
			DocumentType: "DNI", DocumentNumber: target.DocNumber,
			Password: goodPassword,
		}, auth.ClientInfo{})
	}
	session, err := login()
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	employee, err := e.staff.Deactivate(e.ctx, actor, target.ID)
	if err != nil || employee.IsActive {
		t.Fatalf("deactivate: %+v, %v", employee, err)
	}
	_, err = e.auth.Refresh(e.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	_, err = login()
	wantCode(t, err, "INVALID_CREDENTIALS")
	if n := e.countAudit(t, target.ID, "DEACTIVATED"); n != 1 {
		t.Fatalf("expected 1 DEACTIVATED audit row, got %d", n)
	}

	_, err = e.staff.Deactivate(e.ctx, actor, target.ID)
	wantCode(t, err, "EMPLOYEE_ALREADY_INACTIVE")

	employee, err = e.staff.Reactivate(e.ctx, actor, target.ID)
	if err != nil || !employee.IsActive {
		t.Fatalf("reactivate: %+v, %v", employee, err)
	}
	if _, err := login(); err != nil {
		t.Fatalf("login after reactivation: %v", err)
	}
	_, err = e.staff.Reactivate(e.ctx, actor, target.ID)
	wantCode(t, err, "EMPLOYEE_ALREADY_ACTIVE")
	if n := e.countAudit(t, target.ID, "REACTIVATED"); n != 1 {
		t.Fatalf("expected 1 REACTIVATED audit row, got %d", n)
	}

	_, err = e.staff.Deactivate(e.ctx, actor, admin.ID)
	wantCode(t, err, "CANNOT_DEACTIVATE_SELF")
	_, err = e.staff.Deactivate(e.ctx, actor, uuid.New())
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
}

func TestTheLastActiveAdminCannotBeDemoted(t *testing.T) {
	e := newTestEnv(t)
	a := e.insertEmployee(t, "system_admin", goodPassword, "AdminA")
	b := e.insertEmployee(t, "system_admin", goodPassword, "AdminB")

	var others int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM employees
		WHERE role = 'system_admin' AND is_active AND NOT is_system AND id NOT IN ($1, $2)`, a.ID, b.ID).Scan(&others)
	if err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if others > 0 {
		t.Skip("other active admins exist in the test database; this test needs to be the only ones")
	}

	actor := actorOf(a.ID, auth.RoleSystemAdmin)
	if _, err := e.staff.Deactivate(e.ctx, actor, b.ID); err != nil {
		t.Fatalf("deactivating one of two admins must work: %v", err)
	}
	current, err := e.staff.Get(e.ctx, actor, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	role := "sales_manager"
	_, err = e.staff.Update(e.ctx, actor, a.ID, UpdateInput{Version: &current.Version, Role: &role})
	wantCode(t, err, "LAST_ADMIN_PROTECTED")
}

// ---- delete ------------------------------------------------------------------------------------

func TestDeleteOnlyRemovesEmployeesWithoutHistory(t *testing.T) {
	e := newTestEnv(t)
	admin := e.insertEmployee(t, "system_admin", goodPassword, "Admin")
	actor := actorOf(admin.ID, auth.RoleSystemAdmin)

	created, _ := e.createViaService(t, actor)
	id := created.Employee.ID

	wantCode(t, e.staff.Delete(e.ctx, actor, id, "  "), "REASON_REQUIRED")
	wantCode(t, e.staff.Delete(e.ctx, actor, admin.ID, "prueba"), "CANNOT_DELETE_SELF")
	if err := e.staff.Delete(e.ctx, actor, id, "Registrado por error"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := e.staff.Get(e.ctx, actor, id)
	wantCode(t, err, "EMPLOYEE_NOT_FOUND")
	if n := e.countAudit(t, id, "DELETED"); n != 1 {
		t.Fatalf("expected 1 DELETED audit row (the audit trail survives), got %d", n)
	}

	// Someone who signed in has history and is protected.
	used := e.insertEmployee(t, "sales_advisor", goodPassword, "ConHistorial")
	if _, err := e.auth.Login(e.ctx, auth.LoginRequest{
		DocumentType: "DNI", DocumentNumber: used.DocNumber,
		Password: goodPassword,
	}, auth.ClientInfo{}); err != nil {
		t.Fatalf("login: %v", err)
	}
	wantCode(t, e.staff.Delete(e.ctx, actor, used.ID, "intento"), "EMPLOYEE_HAS_HISTORY")
}

// ---- activation, pickers and lists -------------------------------------------------------------

func TestResendActivationUntilTheAccountIsActivated(t *testing.T) {
	e := newTestEnv(t)
	actor := actorOf(e.insertEmployee(t, "system_admin", goodPassword, "Admin").ID, auth.RoleSystemAdmin)
	created, in := e.createViaService(t, actor)

	if err := e.staff.ResendActivation(e.ctx, actor, created.Employee.ID); err != nil {
		t.Fatalf("resend: %v", err)
	}
	e.auth.WaitForBackgroundWork()
	sent := e.mail.to(in.BusinessEmail)
	if len(sent) != 2 {
		t.Fatalf("expected two activation emails, got %d", len(sent))
	}
	match := tokenInText.FindStringSubmatch(sent[1].Body)
	if match == nil {
		t.Fatal("the second email must contain a link")
	}
	if err := e.auth.Activate(e.ctx, match[1], goodPassword, auth.ClientInfo{}); err != nil {
		t.Fatalf("activate with the newest link: %v", err)
	}
	wantCode(t, e.staff.ResendActivation(e.ctx, actor, created.Employee.ID), "EMPLOYEE_ALREADY_ACTIVATED")
}

func TestAdvisorsListsOnlyActiveSalesAdvisors(t *testing.T) {
	e := newTestEnv(t)
	admin := actorOf(uuid.New(), auth.RoleSystemAdmin)

	active := e.insertEmployee(t, "sales_advisor", "", "AsesorActivo")
	inactive := e.insertEmployee(t, "sales_advisor", "", "AsesorInactivo")
	manager := e.insertEmployee(t, "sales_manager", "", "Gerente")
	if _, err := e.pool.Exec(e.ctx, `UPDATE employees SET is_active = false, deactivated_at = now() WHERE id = $1`, inactive.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	for _, caller := range []auth.Actor{admin, actorOf(uuid.New(), auth.RoleSalesManager)} {
		advisors, err := e.staff.Advisors(e.ctx, caller)
		if err != nil {
			t.Fatalf("advisors: %v", err)
		}
		found := map[uuid.UUID]bool{}
		for _, a := range advisors {
			found[a.ID] = true
		}
		if !found[active.ID] || found[inactive.ID] || found[manager.ID] {
			t.Fatalf("unexpected advisors for %s", caller.Role)
		}
	}
	_, err := e.staff.Advisors(e.ctx, actorOf(uuid.New(), auth.RoleSalesAdvisor))
	wantCode(t, err, "FORBIDDEN")
}

func TestListSearchesFiltersAndPaginates(t *testing.T) {
	e := newTestEnv(t)
	admin := actorOf(uuid.New(), auth.RoleSystemAdmin)
	marker := "Zq" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]

	e.insertEmployee(t, "sales_advisor", "", marker+"A")
	e.insertEmployee(t, "sales_advisor", "", marker+"B")
	inactive := e.insertEmployee(t, "sales_manager", "", marker+"C")
	if _, err := e.pool.Exec(e.ctx, `UPDATE employees SET is_active = false, deactivated_at = now() WHERE id = $1`, inactive.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	all, err := e.staff.List(e.ctx, admin, ListFilter{Search: strings.ToLower(marker)})
	if err != nil || all.Total != 3 {
		t.Fatalf("search must be case-insensitive and find 3: %+v, %v", all, err)
	}
	first, _ := e.staff.List(e.ctx, admin, ListFilter{Search: marker, Page: 1, PageSize: 2})
	second, _ := e.staff.List(e.ctx, admin, ListFilter{Search: marker, Page: 2, PageSize: 2})
	if len(first.Items) != 2 || len(second.Items) != 1 || first.Total != 3 {
		t.Fatalf("unexpected pages: %d, %d, total %d", len(first.Items), len(second.Items), first.Total)
	}
	advisors, _ := e.staff.List(e.ctx, admin, ListFilter{Search: marker, Role: "sales_advisor"})
	if advisors.Total != 2 {
		t.Fatalf("role filter: got %d, want 2", advisors.Total)
	}
	inactiveOnly, _ := e.staff.List(e.ctx, admin, ListFilter{Search: marker, Status: "inactive"})
	if inactiveOnly.Total != 1 || inactiveOnly.Items[0].ID != inactive.ID {
		t.Fatalf("status filter: %+v", inactiveOnly)
	}
	byDocument, _ := e.staff.List(e.ctx, admin, ListFilter{Search: inactive.DocNumber})
	if byDocument.Total != 1 {
		t.Fatalf("search by document number: got %d, want 1", byDocument.Total)
	}
}

func TestOnlyAdminsManageStaff(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.New()
	version := 1
	for _, role := range []auth.Role{auth.RoleSalesManager, auth.RoleSalesAdvisor} {
		actor := actorOf(uuid.New(), role)
		_, listErr := e.staff.List(e.ctx, actor, ListFilter{})
		_, getErr := e.staff.Get(e.ctx, actor, id)
		_, createErr := e.staff.Create(e.ctx, actor, validInput())
		_, updateErr := e.staff.Update(e.ctx, actor, id, UpdateInput{Version: &version})
		_, deactivateErr := e.staff.Deactivate(e.ctx, actor, id)
		_, reactivateErr := e.staff.Reactivate(e.ctx, actor, id)
		resendErr := e.staff.ResendActivation(e.ctx, actor, id)
		deleteErr := e.staff.Delete(e.ctx, actor, id, "motivo")
		for name, err := range map[string]error{
			"list": listErr, "get": getErr, "create": createErr, "update": updateErr,
			"deactivate": deactivateErr, "reactivate": reactivateErr, "resend": resendErr, "delete": deleteErr,
		} {
			if code(err) != "FORBIDDEN" {
				t.Errorf("%s as %s: got %v, want FORBIDDEN", name, role, err)
			}
		}
	}
}
