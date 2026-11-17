package auth

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests run against a migrated database: run `make db-verify` once, then `make test`.
// They create their own employees and remove them afterwards.

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// queueRecorder stands in for the notifications writer: it records what was queued and writes a real
// notification_logs row, which is what the reset cooldown counts. (The auth package cannot import the real
// writer, which depends on it.)
type queueRecorder struct {
	mu     sync.Mutex
	queued []queuedEmail
}

type queuedEmail struct {
	Kind       string // employee_activation or password_reset
	EmployeeID uuid.UUID
	Email      string
}

func (q *queueRecorder) add(ctx context.Context, tx pgx.Tx, kind string, id uuid.UUID, email string) error {
	q.mu.Lock()
	q.queued = append(q.queued, queuedEmail{kind, id, email})
	q.mu.Unlock()
	_, err := tx.Exec(ctx, `INSERT INTO notification_logs
		(id, notification_type, recipient_employee_id, recipient_email, dedupe_key)
		VALUES ($1, $2::text::notification_type, $3, $4, $5)`, ids.New(), kind, id, email, kind+":"+ids.NewString())
	return err
}

func (q *queueRecorder) EmployeeActivation(ctx context.Context, tx pgx.Tx, id uuid.UUID, email string) error {
	return q.add(ctx, tx, "employee_activation", id, email)
}

func (q *queueRecorder) PasswordReset(ctx context.Context, tx pgx.Tx, id uuid.UUID, email string) error {
	return q.add(ctx, tx, "password_reset", id, email)
}

func (q *queueRecorder) of(kind string, id uuid.UUID) []queuedEmail {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []queuedEmail
	for _, e := range q.queued {
		if e.Kind == kind && e.EmployeeID == id {
			out = append(out, e)
		}
	}
	return out
}

func (q *queueRecorder) total() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queued)
}

type testEnv struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	svc   *Service
	clock *fakeClock
	queue *queueRecorder
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

	clock := &fakeClock{now: time.Now().UTC().Truncate(time.Second)}
	queue := &queueRecorder{}
	svc := NewService(Deps{
		DB: pool, JWTSecret: string(testSecret), AppBaseURL: "http://app.test", PortalBaseURL: "https://portal.test",
		Notifier: queue, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: clock.Now,
	})
	return &testEnv{ctx: ctx, pool: pool, svc: svc, clock: clock, queue: queue}
}

type testEmployee struct {
	ID        uuid.UUID
	DocType   string
	DocNumber string
	Password  string
	Email     string
}

const goodPassword = "Correct-Horse-Battery-9"

// createEmployee inserts an employee; an empty password means "not activated yet".
func (e *testEnv) createEmployee(t *testing.T, role Role, password string) testEmployee {
	t.Helper()
	id := ids.New()
	emp := testEmployee{
		ID: id, DocType: "DNI", DocNumber: fmt.Sprintf("%08d", rand.IntN(100_000_000)),
		Password: password, Email: fmt.Sprintf("test-%s@test.pe", id),
	}
	var hash *string
	var activated *time.Time
	if password != "" {
		h, err := HashPassword(password)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		hash = &h
		now := e.clock.Now()
		activated = &now
	}
	_, err := e.pool.Exec(e.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, password_hash, activated_at, created_by)
		VALUES ($1, 'DNI', $2, 'Prueba', 'Auth', 'Peruana', 'M', DATE '1990-01-01', DATE '2020-01-01',
				'999000000', $3, $4::text::employee_role, $5, $6, $7)`,
		id, emp.DocNumber, emp.Email, string(role), hash, activated, SystemActorID)
	if err != nil {
		t.Fatalf("insert employee: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = e.pool.Exec(ctx, `DELETE FROM employee_refresh_tokens WHERE employee_id = $1`, id)
		_, _ = e.pool.Exec(ctx, `DELETE FROM account_tokens WHERE employee_id = $1`, id)
		_, _ = e.pool.Exec(ctx, `DELETE FROM notification_logs WHERE recipient_employee_id = $1`, id)
		_, _ = e.pool.Exec(ctx, `DELETE FROM employees WHERE id = $1`, id)
	})
	return emp
}

func (e *testEnv) login(emp testEmployee, password string, remember bool) (*Session, error) {
	return e.svc.Login(e.ctx, LoginRequest{
		DocumentType: emp.DocType, DocumentNumber: emp.DocNumber, Password: password, RememberMe: remember,
	}, ClientInfo{DeviceInfo: "test", IP: "127.0.0.1"})
}

func (e *testEnv) mustLogin(t *testing.T, emp testEmployee, remember bool) *Session {
	t.Helper()
	session, err := e.login(emp, emp.Password, remember)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return session
}

func (e *testEnv) countAccessLogs(t *testing.T, id uuid.UUID, action string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM access_logs
		WHERE employee_id = $1 AND action_type = $2::text::access_action`, id, action).Scan(&n)
	if err != nil {
		t.Fatalf("count access logs: %v", err)
	}
	return n
}

func errCode(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if got := errCode(err); got != code {
		t.Fatalf("got error %v (code %q), want code %q", err, got, code)
	}
}

var tokenInText = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

func tokenFrom(t *testing.T, text string) string {
	t.Helper()
	match := tokenInText.FindStringSubmatch(text)
	if match == nil {
		t.Fatalf("no token found in %q", text)
	}
	return match[1]
}

// issueLink does what the dispatcher does just before sending: mint the link of an account email.
func (e *testEnv) issueLink(t *testing.T, kind LinkKind, empID uuid.UUID) (AccountLink, string) {
	t.Helper()
	var link AccountLink
	var skipped string
	err := database.WithTx(e.ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		link, skipped, err = e.svc.IssueLink(e.ctx, tx, kind, LinkRecipient{EmployeeID: &empID})
		return err
	})
	if err != nil {
		t.Fatalf("issue link: %v", err)
	}
	return link, skipped
}

// queueActivation queues an activation email the way staff management does.
func (e *testEnv) queueActivation(empID uuid.UUID) error {
	return database.WithTx(e.ctx, e.pool, func(tx pgx.Tx) error { return e.svc.QueueActivation(e.ctx, tx, empID) })
}

// ---- login -------------------------------------------------------------------------------------

func TestLoginSucceedsAndStartsASession(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	session, err := env.login(emp, goodPassword, false)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	claims, err := ParseAccessToken(testSecret, session.AccessToken, env.clock.Now())
	if err != nil || claims.EmployeeID != emp.ID || claims.Role != RoleSalesAdvisor {
		t.Fatalf("unexpected access token: %+v, %v", claims, err)
	}
	if session.RefreshToken == "" || session.Employee.DocumentNumber != emp.DocNumber {
		t.Fatalf("unexpected session: %+v", session)
	}
	if n := env.countAccessLogs(t, emp.ID, "login_success"); n != 1 {
		t.Fatalf("expected 1 login_success row, got %d", n)
	}
}

func TestAWrongPasswordAndAnUnknownAccountLookTheSame(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	_, wrongPassword := env.login(emp, "wrong password 123", false)
	_, unknown := env.svc.Login(env.ctx, LoginRequest{DocumentType: "DNI", DocumentNumber: "00000000", Password: "whatever-123"}, ClientInfo{})

	wantCode(t, wrongPassword, "INVALID_CREDENTIALS")
	wantCode(t, unknown, "INVALID_CREDENTIALS")
	if wrongPassword.Error() != unknown.Error() && apperr.FromError(wrongPassword).Message != apperr.FromError(unknown).Message {
		t.Fatal("the client must not be able to tell the two apart")
	}
}

func TestANotYetActivatedAccountCannotLogIn(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, "")
	_, err := env.login(emp, "anything-at-all-1", false)
	wantCode(t, err, "INVALID_CREDENTIALS")
}

func TestFiveFailedLoginsLockTheEmployeeAccount(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	for i := 0; i < MaxFailedLogins; i++ {
		_, err := env.login(emp, "wrong password 123", false)
		wantCode(t, err, "INVALID_CREDENTIALS")
		env.clock.Advance(time.Minute)
	}
	_, err := env.login(emp, goodPassword, false)
	wantCode(t, err, "INVALID_CREDENTIALS") // even the right password is refused while locked

	env.clock.Advance(LockoutDuration + time.Minute)
	if _, err := env.login(emp, goodPassword, false); err != nil {
		t.Fatalf("the lock must end: %v", err)
	}
}

func TestFailuresSpreadOverTimeDoNotLockTheAccount(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	for round := 0; round < 3; round++ {
		for i := 0; i < MaxFailedLogins-1; i++ {
			_, _ = env.login(emp, "wrong password 123", false)
		}
		env.clock.Advance(FailureWindow + time.Minute)
	}
	if _, err := env.login(emp, goodPassword, false); err != nil {
		t.Fatalf("slow failures must not lock the account: %v", err)
	}
}

func TestASuccessfulLoginClearsTheFailureCount(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	for i := 0; i < MaxFailedLogins-1; i++ {
		_, _ = env.login(emp, "wrong password 123", false)
	}
	if _, err := env.login(emp, goodPassword, false); err != nil {
		t.Fatalf("login: %v", err)
	}
	for i := 0; i < MaxFailedLogins-1; i++ {
		_, _ = env.login(emp, "wrong password 123", false)
	}
	if _, err := env.login(emp, goodPassword, false); err != nil {
		t.Fatalf("the count must have restarted after the success: %v", err)
	}
}

func TestADeactivatedEmployeeCannotLogInOrRefreshOrAuthenticate(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, false)

	if _, err := env.pool.Exec(env.ctx, `UPDATE employees SET is_active = false, deactivated_at = now() WHERE id = $1`, emp.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	_, err := env.svc.Refresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	_, err = env.login(emp, goodPassword, false)
	wantCode(t, err, "INVALID_CREDENTIALS")
	_, err = env.svc.Authenticate(env.ctx, session.AccessToken)
	wantCode(t, err, "ACCESS_TOKEN_INVALID")
}

// ---- sessions ----------------------------------------------------------------------------------

func TestRefreshRotatesTheToken(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	first := env.mustLogin(t, emp, true)

	env.clock.Advance(time.Hour)
	second, err := env.svc.Refresh(env.ctx, first.RefreshToken)
	if err != nil || second.RefreshToken == "" || second.RefreshToken == first.RefreshToken {
		t.Fatalf("expected a new refresh token: %+v, %v", second, err)
	}
	env.clock.Advance(time.Hour)
	if _, err := env.svc.Refresh(env.ctx, second.RefreshToken); err != nil {
		t.Fatalf("the rotated token must work: %v", err)
	}
}

func TestReplayingARotatedTokenRevokesTheWholeFamily(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	first := env.mustLogin(t, emp, true)

	second, err := env.svc.Refresh(env.ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	env.clock.Advance(refreshGrace + time.Second)

	_, err = env.svc.Refresh(env.ctx, first.RefreshToken) // the old token again: looks stolen
	wantCode(t, err, "SESSION_INVALID")
	_, err = env.svc.Refresh(env.ctx, second.RefreshToken) // its whole family is now dead
	wantCode(t, err, "SESSION_INVALID")
}

func TestReplayInsideTheGraceWindowReturnsOnlyAnAccessToken(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	first := env.mustLogin(t, emp, true)

	second, err := env.svc.Refresh(env.ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	env.clock.Advance(5 * time.Second) // the second tab's request arrives a moment later

	replay, err := env.svc.Refresh(env.ctx, first.RefreshToken)
	if err != nil || replay.AccessToken == "" || replay.RefreshToken != "" {
		t.Fatalf("expected an access token and no new refresh token: %+v, %v", replay, err)
	}
	if _, err := env.svc.Refresh(env.ctx, second.RefreshToken); err != nil {
		t.Fatalf("the session must survive the race: %v", err)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, false)

	if err := env.svc.Logout(env.ctx, session.RefreshToken, ClientInfo{}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	_, err := env.svc.Refresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	if n := env.countAccessLogs(t, emp.ID, "logout"); n != 1 {
		t.Fatalf("expected 1 logout row, got %d", n)
	}
	if err := env.svc.Logout(env.ctx, "unknown-token", ClientInfo{}); err != nil {
		t.Fatalf("logging out an unknown token must not fail: %v", err)
	}
}

func TestASessionWithoutRememberMeEndsAfterTwelveHours(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, false)

	env.clock.Advance(refreshSessionTTL + time.Second)
	_, err := env.svc.Refresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
}

func TestARememberedSessionSlidesButHasAnAbsoluteCap(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	token := env.mustLogin(t, emp, true).RefreshToken

	for i := 0; i < 3; i++ { // days 29, 58 and 87: each refresh slides the 30-day window
		env.clock.Advance(29 * 24 * time.Hour)
		session, err := env.svc.Refresh(env.ctx, token)
		if err != nil {
			t.Fatalf("refresh %d: %v", i+1, err)
		}
		token = session.RefreshToken
	}
	env.clock.Advance(29 * 24 * time.Hour) // day 116: past the 90-day cap
	_, err := env.svc.Refresh(env.ctx, token)
	wantCode(t, err, "SESSION_INVALID")
}

// ---- authenticate ------------------------------------------------------------------------------

func TestAuthenticateUsesTheRoleFromTheDatabase(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, false)

	actor, err := env.svc.Authenticate(env.ctx, session.AccessToken)
	if err != nil || actor.ID != emp.ID || actor.Role != RoleSalesAdvisor {
		t.Fatalf("unexpected actor: %+v, %v", actor, err)
	}
	if _, err := env.pool.Exec(env.ctx, `UPDATE employees SET role = 'sales_manager' WHERE id = $1`, emp.ID); err != nil {
		t.Fatalf("change role: %v", err)
	}
	actor, err = env.svc.Authenticate(env.ctx, session.AccessToken)
	if err != nil || actor.Role != RoleSalesManager {
		t.Fatalf("a role change must apply on the next request: %+v, %v", actor, err)
	}
}

func TestAnExpiredAccessTokenIsRejected(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, false)

	env.clock.Advance(AccessTokenTTL + time.Second)
	_, err := env.svc.Authenticate(env.ctx, session.AccessToken)
	wantCode(t, err, "ACCESS_TOKEN_INVALID")
}

// ---- activation and recovery -------------------------------------------------------------------

func TestActivationFlow(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, "")

	if err := env.queueActivation(emp.ID); err != nil {
		t.Fatalf("queue activation: %v", err)
	}
	if got := env.queue.of("employee_activation", emp.ID); len(got) != 1 || got[0].Email != emp.Email {
		t.Fatalf("expected one activation email queued to the business address, got %+v", got)
	}

	link, skipped := env.issueLink(t, LinkActivation, emp.ID)
	if skipped != "" {
		t.Fatalf("a link must be issued, skipped: %s", skipped)
	}
	parsed, err := url.Parse(link.URL)
	if err != nil || parsed.Path != "/activate-account" || link.ExpiresIn != activationTTL {
		t.Fatalf("unexpected link %+v", link)
	}
	token := parsed.Query().Get("token")

	_, err = env.svc.Activate(env.ctx, token, "corta1", ClientInfo{})
	wantCode(t, err, "PASSWORD_TOO_WEAK") // the link must survive a weak password
	account, err := env.svc.Activate(env.ctx, token, goodPassword, ClientInfo{})
	if err != nil || account != AccountEmployee {
		t.Fatalf("activate: %v (account %q)", err, account)
	}
	if _, err := env.login(emp, goodPassword, false); err != nil {
		t.Fatalf("login after activation: %v", err)
	}
	_, err = env.svc.Activate(env.ctx, token, goodPassword, ClientInfo{})
	wantCode(t, err, "INVALID_LINK") // single use

	wantCode(t, env.queueActivation(emp.ID), "EMPLOYEE_ALREADY_ACTIVATED")
	if _, skipped := env.issueLink(t, LinkActivation, emp.ID); skipped == "" {
		t.Fatal("no link may be minted for an account that is already active")
	}
}

func TestANewLinkReplacesTheOpenOne(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, "")
	first, _ := env.issueLink(t, LinkActivation, emp.ID)
	second, _ := env.issueLink(t, LinkActivation, emp.ID) // a retry or a resend

	_, err := env.svc.Activate(env.ctx, tokenFrom(t, first.URL), goodPassword, ClientInfo{})
	wantCode(t, err, "INVALID_LINK")
	if _, err := env.svc.Activate(env.ctx, tokenFrom(t, second.URL), goodPassword, ClientInfo{}); err != nil {
		t.Fatalf("the newest link must work: %v", err)
	}
}

func TestAnExpiredActivationLinkIsRejected(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, "")
	link, _ := env.issueLink(t, LinkActivation, emp.ID)

	env.clock.Advance(activationTTL + time.Second)
	_, err := env.svc.Activate(env.ctx, tokenFrom(t, link.URL), goodPassword, ClientInfo{})
	wantCode(t, err, "INVALID_LINK")
}

func TestPasswordResetFlow(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, true)

	if err := env.svc.ForgotPassword(env.ctx, emp.DocType, emp.DocNumber, ClientInfo{}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if got := env.queue.of("password_reset", emp.ID); len(got) != 1 || got[0].Email != emp.Email {
		t.Fatalf("expected one reset email queued, got %+v", got)
	}
	link, skipped := env.issueLink(t, LinkPasswordReset, emp.ID)
	if skipped != "" || link.ExpiresIn != resetTTL {
		t.Fatalf("unexpected link %+v (skipped %q)", link, skipped)
	}
	token := tokenFrom(t, link.URL)
	if !strings.Contains(link.URL, "/reset-password?token=") {
		t.Fatalf("unexpected reset address %q", link.URL)
	}

	const newPassword = "Another-Strong-Pass-7"
	if err := env.svc.ResetPassword(env.ctx, token, newPassword, ClientInfo{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_, err := env.svc.Refresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID") // every old session ended
	_, err = env.login(emp, goodPassword, false)
	wantCode(t, err, "INVALID_CREDENTIALS")
	if _, err := env.login(emp, newPassword, false); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	wantCode(t, env.svc.ResetPassword(env.ctx, token, newPassword, ClientInfo{}), "INVALID_LINK") // single use
}

func TestAResetLinkCannotActivateAnAccountAndViceVersa(t *testing.T) {
	env := newTestEnv(t)
	active := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	reset, _ := env.issueLink(t, LinkPasswordReset, active.ID)
	_, err := env.svc.Activate(env.ctx, tokenFrom(t, reset.URL), goodPassword, ClientInfo{})
	wantCode(t, err, "INVALID_LINK")

	pending := env.createEmployee(t, RoleSalesAdvisor, "")
	activation, _ := env.issueLink(t, LinkActivation, pending.ID)
	wantCode(t, env.svc.ResetPassword(env.ctx, tokenFrom(t, activation.URL), goodPassword, ClientInfo{}), "INVALID_LINK")
}

func TestForgotPasswordForAnUnknownAccountQueuesNothingAndDoesNotFail(t *testing.T) {
	env := newTestEnv(t)
	if err := env.svc.ForgotPassword(env.ctx, "DNI", "00000000", ClientInfo{}); err != nil {
		t.Fatalf("must succeed silently: %v", err)
	}
	if err := env.svc.ForgotPassword(env.ctx, "XXX", "not a document", ClientInfo{}); err != nil {
		t.Fatalf("must succeed silently: %v", err)
	}
	if n := env.queue.total(); n != 0 {
		t.Fatalf("nothing must be queued, got %d", n)
	}
}

func TestForgotPasswordHasACooldown(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)

	for i := 0; i < 3; i++ {
		if err := env.svc.ForgotPassword(env.ctx, emp.DocType, emp.DocNumber, ClientInfo{}); err != nil {
			t.Fatalf("forgot password: %v", err)
		}
	}
	if n := len(env.queue.of("password_reset", emp.ID)); n != 1 {
		t.Fatalf("expected a single email inside the cooldown, got %d", n)
	}
	env.clock.Advance(resetCooldown + time.Second)
	if err := env.svc.ForgotPassword(env.ctx, emp.DocType, emp.DocNumber, ClientInfo{}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if n := len(env.queue.of("password_reset", emp.ID)); n != 2 {
		t.Fatalf("expected a second email after the cooldown, got %d", n)
	}
}

func TestChangePassword(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	session := env.mustLogin(t, emp, true)
	const newPassword = "Brand-New-Password-5"

	wantCode(t, env.svc.ChangePassword(env.ctx, emp.ID, "not the current one", newPassword, ClientInfo{}), "INVALID_CURRENT_PASSWORD")
	wantCode(t, env.svc.ChangePassword(env.ctx, emp.ID, goodPassword, "corta1", ClientInfo{}), "PASSWORD_TOO_WEAK")
	if err := env.svc.ChangePassword(env.ctx, emp.ID, goodPassword, newPassword, ClientInfo{}); err != nil {
		t.Fatalf("change password: %v", err)
	}
	_, err := env.svc.Refresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	if _, err := env.login(emp, newPassword, false); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	if n := env.countAccessLogs(t, emp.ID, "password_changed"); n != 1 {
		t.Fatalf("expected 1 password_changed row, got %d", n)
	}
}
