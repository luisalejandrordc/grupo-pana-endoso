package auth

import (
	"endosoflow/internal/platform/database"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The customer portal's sign-in follows the employee flow, so these tests mirror the employee ones and add what
// only matters between the two kinds of account: a customer's token must never open an employee route, nor an
// employee's token a customer route.

type testCustomer struct {
	ID       uuid.UUID
	DocType  string
	DocNum   string
	Email    string
	Password string
}

// insertActiveCustomer adds an activated customer who can sign in.
func (e *testEnv) insertActiveCustomer(t *testing.T) testCustomer {
	t.Helper()
	id, email := e.insertCustomer(t)
	var number string
	if err := e.pool.QueryRow(e.ctx, `SELECT document_number FROM customers WHERE id = $1`, id).Scan(&number); err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword(goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE customers SET password_hash = $2, activated_at = now() WHERE id = $1`, id, hash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(e.ctx, `DELETE FROM customer_refresh_tokens WHERE customer_id = $1`, id) })
	return testCustomer{ID: id, DocType: "DNI", DocNum: number, Email: email, Password: goodPassword}
}

func (e *testEnv) customerLogin(c testCustomer, password string, remember bool) (*CustomerSession, error) {
	return e.svc.CustomerLogin(e.ctx, LoginRequest{
		DocumentType: c.DocType, DocumentNumber: c.DocNum, Password: password, RememberMe: remember,
	}, ClientInfo{DeviceInfo: "test", IP: "127.0.0.1"})
}

func (e *testEnv) mustCustomerLogin(t *testing.T, c testCustomer, remember bool) *CustomerSession {
	t.Helper()
	session, err := e.customerLogin(c, c.Password, remember)
	if err != nil {
		t.Fatalf("customer login: %v", err)
	}
	return session
}

func TestCustomerLoginStartsASessionThatOnlyCustomerRoutesAccept(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	session := env.mustCustomerLogin(t, c, true)
	if session.Customer.ID != c.ID || session.RefreshToken == "" || session.Customer.DisplayName == "" {
		t.Fatalf("unexpected session %+v", session)
	}

	actor, err := env.svc.AuthenticateCustomer(env.ctx, session.AccessToken)
	if err != nil || actor.ID != c.ID {
		t.Fatalf("a customer's token must authenticate the customer: %+v, %v", actor, err)
	}
	// The customer's token is not an employee's.
	if _, err := env.svc.Authenticate(env.ctx, session.AccessToken); err == nil {
		t.Fatal("a customer's access token must not open an employee route")
	}
	// And the other way round.
	emp := env.createEmployee(t, RoleSystemAdmin, goodPassword)
	employeeSession := env.mustLogin(t, emp, false)
	if _, err := env.svc.AuthenticateCustomer(env.ctx, employeeSession.AccessToken); err == nil {
		t.Fatal("an employee's access token must not open a customer route")
	}
	if n := env.countCustomerAccessLogs(t, c.ID, "login_success"); n != 1 {
		t.Fatalf("the login must be logged against the customer, got %d", n)
	}
}

func (e *testEnv) countCustomerAccessLogs(t *testing.T, id uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM access_logs
		WHERE customer_id = $1 AND account_type = 'customer' AND action_type = $2::text::access_action`, id, action).Scan(&n); err != nil {
		t.Fatalf("count access logs: %v", err)
	}
	return n
}

func TestCustomerLoginAcceptsEveryDocumentTypeFormat(t *testing.T) {
	for _, tc := range []struct {
		docType, number string
		ok              bool
	}{
		{"DNI", "12345678", true},
		{"dni", " 12345678 ", true},
		{"RUC", "20123456789", true},
		{"CE", "ab1234567", true},
		{"PAS", "x1234567", true},
		{"CDI", "ABCD12345", true},
		{"DNI", "1234567", false},
		{"RUC", "2012345678", false},
		{"XXX", "12345678", false},
		{"", "", false},
	} {
		_, number, valid := NormalizeCustomerIdentifier(tc.docType, tc.number)
		if valid != tc.ok {
			t.Errorf("%q %q: valid = %v, want %v (normalized %q)", tc.docType, tc.number, valid, tc.ok, number)
		}
	}
}

func TestACustomerAndAnEmployeeWithTheSameDocumentDoNotMixUp(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	// An employee who happens to hold the same DNI, with a different password.
	if _, err := env.pool.Exec(env.ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, created_by)
		VALUES ($1, 'DNI', $2, 'Ana', 'Doble', 'Peruana', 'F', DATE '1990-01-01', DATE '2026-01-01',
				'999000111', $3, 'sales_advisor', $4)`,
		uuid.New(), c.DocNum, "doble-"+c.Email, SystemActorID); err != nil {
		t.Skipf("the shared document could not be inserted (%v)", err)
	}
	t.Cleanup(func() {
		_, _ = env.pool.Exec(env.ctx, `DELETE FROM access_logs WHERE attempted_document_number = $1`, c.DocNum)
		_, _ = env.pool.Exec(env.ctx, `DELETE FROM employees WHERE document_number = $1 AND business_email = $2`, c.DocNum, "doble-"+c.Email)
	})
	if _, err := env.customerLogin(c, c.Password, false); err != nil {
		t.Fatalf("the customer signs in with their own password: %v", err)
	}
	// The employee is not activated, and the customer endpoint never looks at employees.
	_, err := env.svc.Login(env.ctx, LoginRequest{DocumentType: "DNI", DocumentNumber: c.DocNum, Password: c.Password}, ClientInfo{})
	wantCode(t, err, "INVALID_CREDENTIALS")
}

func TestCustomerFailuresAllLookTheSame(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	_, err := env.customerLogin(c, "nope-nope-nope-1", false)
	wantCode(t, err, "INVALID_CREDENTIALS")
	_, err = env.svc.CustomerLogin(env.ctx, LoginRequest{DocumentType: "DNI", DocumentNumber: "00000000", Password: goodPassword}, ClientInfo{})
	wantCode(t, err, "INVALID_CREDENTIALS")
	_, err = env.svc.CustomerLogin(env.ctx, LoginRequest{DocumentType: "ZZZ", DocumentNumber: "x", Password: goodPassword}, ClientInfo{})
	wantCode(t, err, "INVALID_CREDENTIALS")

	notActivated, _ := env.insertCustomer(t) // no password yet
	var number string
	_ = env.pool.QueryRow(env.ctx, `SELECT document_number FROM customers WHERE id = $1`, notActivated).Scan(&number)
	_, err = env.svc.CustomerLogin(env.ctx, LoginRequest{DocumentType: "DNI", DocumentNumber: number, Password: goodPassword}, ClientInfo{})
	wantCode(t, err, "INVALID_CREDENTIALS")
	var reason string
	if err := env.pool.QueryRow(env.ctx, `SELECT failure_reason::text FROM access_logs WHERE customer_id = $1 ORDER BY created_at DESC LIMIT 1`, notActivated).Scan(&reason); err != nil || reason != "not_activated" {
		t.Fatalf("the reason goes to the log, not to the caller: %q (%v)", reason, err)
	}
}

func TestFiveFailedCustomerLoginsLockTheAccountAndItUnlocksLater(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	for i := 0; i < MaxFailedLogins; i++ {
		_, err := env.customerLogin(c, "wrong-wrong-wrong-1", false)
		wantCode(t, err, "INVALID_CREDENTIALS")
	}
	_, err := env.customerLogin(c, c.Password, false) // even the right password is refused while locked
	wantCode(t, err, "INVALID_CREDENTIALS")

	env.clock.Advance(LockoutDuration + time.Second)
	if _, err := env.customerLogin(c, c.Password, false); err != nil {
		t.Fatalf("the lock must expire: %v", err)
	}
}

func TestCustomerRefreshRotatesAndReplayKillsTheFamily(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	first := env.mustCustomerLogin(t, c, true)

	second, err := env.svc.CustomerRefresh(env.ctx, first.RefreshToken)
	if err != nil || second.RefreshToken == "" || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh must rotate the token: %+v, %v", second, err)
	}

	// Inside the grace window a second tab gets only an access token.
	env.clock.Advance(5 * time.Second)
	replay, err := env.svc.CustomerRefresh(env.ctx, first.RefreshToken)
	if err != nil || replay.AccessToken == "" || replay.RefreshToken != "" {
		t.Fatalf("grace replay: %+v, %v", replay, err)
	}

	// Later it looks stolen: the whole family dies.
	env.clock.Advance(refreshGrace + time.Second)
	_, err = env.svc.CustomerRefresh(env.ctx, first.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	_, err = env.svc.CustomerRefresh(env.ctx, second.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
}

func TestCustomerSessionLifetimes(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)

	short := env.mustCustomerLogin(t, c, false)
	env.clock.Advance(refreshSessionTTL + time.Minute)
	_, err := env.svc.CustomerRefresh(env.ctx, short.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")

	remembered := env.mustCustomerLogin(t, c, true)
	env.clock.Advance(refreshAbsoluteTTL + time.Minute)
	_, err = env.svc.CustomerRefresh(env.ctx, remembered.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
}

func TestCustomerLogoutEndsTheSession(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	session := env.mustCustomerLogin(t, c, false)
	if err := env.svc.CustomerLogout(env.ctx, session.RefreshToken, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	_, err := env.svc.CustomerRefresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	if err := env.svc.CustomerLogout(env.ctx, "unknown-token", ClientInfo{}); err != nil {
		t.Fatalf("an unknown token must not fail: %v", err)
	}
	if env.countCustomerAccessLogs(t, c.ID, "logout") != 1 {
		t.Fatal("the logout must be logged")
	}
}

func TestADeactivatedCustomerLosesTheirSessionsAtOnce(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	session := env.mustCustomerLogin(t, c, true)

	if err := database.WithTx(env.ctx, env.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(env.ctx, `UPDATE customers SET is_active = false, deactivated_at = now(), deactivated_by = created_by WHERE id = $1`, c.ID); err != nil {
			return err
		}
		return env.svc.RevokeCustomerAccess(env.ctx, tx, c.ID, "deactivated", env.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.AuthenticateCustomer(env.ctx, session.AccessToken); err == nil {
		t.Fatal("a deactivated customer's access token must stop working")
	}
	_, err := env.svc.CustomerRefresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	_, err = env.customerLogin(c, c.Password, false)
	wantCode(t, err, "INVALID_CREDENTIALS")
}

func TestChangeCustomerPassword(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	session := env.mustCustomerLogin(t, c, true)

	wantCode(t, env.svc.ChangeCustomerPassword(env.ctx, c.ID, "not-my-password-1", "Brand-New-Password-5", ClientInfo{}), "INVALID_CURRENT_PASSWORD")
	wantCode(t, env.svc.ChangeCustomerPassword(env.ctx, c.ID, goodPassword, "corta", ClientInfo{}), "PASSWORD_TOO_WEAK")

	const next = "Brand-New-Password-5"
	if err := env.svc.ChangeCustomerPassword(env.ctx, c.ID, goodPassword, next, ClientInfo{}); err != nil {
		t.Fatalf("change: %v", err)
	}
	_, err := env.svc.CustomerRefresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID") // every session ended
	_, err = env.customerLogin(c, goodPassword, false)
	wantCode(t, err, "INVALID_CREDENTIALS")
	if _, err := env.customerLogin(c, next, false); err != nil {
		t.Fatalf("the new password must work: %v", err)
	}
	if env.countCustomerAccessLogs(t, c.ID, "password_changed") != 1 {
		t.Fatal("the change must be logged")
	}
}

func TestCustomerPasswordResetFlow(t *testing.T) {
	env := newTestEnv(t)
	c := env.insertActiveCustomer(t)
	session := env.mustCustomerLogin(t, c, true)

	// Nobody is told which documents exist.
	if err := env.svc.CustomerForgotPassword(env.ctx, "DNI", "00000000", ClientInfo{}); err != nil {
		t.Fatalf("must succeed silently: %v", err)
	}
	if err := env.svc.CustomerForgotPassword(env.ctx, "nope", "x", ClientInfo{}); err != nil {
		t.Fatalf("must succeed silently: %v", err)
	}
	if env.queue.total() != 0 {
		t.Fatal("nothing is queued for an unknown account")
	}

	for i := 0; i < 3; i++ { // the cooldown keeps it to one email
		if err := env.svc.CustomerForgotPassword(env.ctx, c.DocType, c.DocNum, ClientInfo{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := env.queue.of("customer_password_reset", c.ID); len(got) != 1 || got[0].Email != c.Email {
		t.Fatalf("one reset email to the customer, got %+v", got)
	}

	// The dispatcher mints the link when it sends: it opens in the customer portal.
	svc := NewService(Deps{
		DB: env.pool, JWTSecret: string(testSecret), AppBaseURL: "http://app.test",
		PortalBaseURL: "https://portal.test", Notifier: env.queue,
	})
	var link AccountLink
	if err := database.WithTx(env.ctx, env.pool, func(tx pgx.Tx) error {
		var skipped string
		var err error
		link, skipped, err = svc.IssueLink(env.ctx, tx, LinkPasswordReset, LinkRecipient{CustomerID: &c.ID})
		if skipped != "" {
			t.Fatalf("skipped: %s", skipped)
		}
		return err
	}); err != nil || !strings.HasPrefix(link.URL, "https://portal.test/reset-password?token=") {
		t.Fatalf("the reset link must open in the customer portal: %q (%v)", link.URL, err)
	}
	token := tokenFrom(t, link.URL)

	// A reset link is not an activation link.
	_, err := env.svc.Activate(env.ctx, token, "Another-Strong-Pass-7", ClientInfo{})
	wantCode(t, err, "INVALID_LINK")
	wantCode(t, env.svc.ResetPassword(env.ctx, token, "corta", ClientInfo{}), "PASSWORD_TOO_WEAK") // the link survives

	const next = "Another-Strong-Pass-7"
	if err := env.svc.ResetPassword(env.ctx, token, next, ClientInfo{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_, err = env.svc.CustomerRefresh(env.ctx, session.RefreshToken)
	wantCode(t, err, "SESSION_INVALID")
	if _, err := env.customerLogin(c, next, false); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	wantCode(t, env.svc.ResetPassword(env.ctx, token, next, ClientInfo{}), "INVALID_LINK") // single use
	if env.countCustomerAccessLogs(t, c.ID, "password_reset_completed") != 1 {
		t.Fatal("the completed reset must be logged")
	}
}

func TestACustomerWhoNeverSetAPasswordGetsNoResetLink(t *testing.T) {
	env := newTestEnv(t)
	id, _ := env.insertCustomer(t)
	var number string
	_ = env.pool.QueryRow(env.ctx, `SELECT document_number FROM customers WHERE id = $1`, id).Scan(&number)
	if err := env.svc.CustomerForgotPassword(env.ctx, "DNI", number, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if env.queue.total() != 0 {
		t.Fatal("a customer without a password has nothing to reset: they need their activation link")
	}
}

func TestAnEmployeeResetLinkStillResetsAnEmployee(t *testing.T) {
	env := newTestEnv(t)
	emp := env.createEmployee(t, RoleSalesAdvisor, goodPassword)
	if err := env.svc.ForgotPassword(env.ctx, emp.DocType, emp.DocNumber, ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	link, _ := env.issueLink(t, LinkPasswordReset, emp.ID)
	if err := env.svc.ResetPassword(env.ctx, tokenFrom(t, link.URL), "Another-Strong-Pass-7", ClientInfo{}); err != nil {
		t.Fatalf("the shared reset endpoint must keep working for employees: %v", err)
	}
}
