package sales_test

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/notifications"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// recorder is the email sender of these tests. It only records, and only misbehaves, for the recipients of the
// sale a test created, because the test database is shared with tests of other packages that queue
// notifications at the same time and that this dispatcher may pick up too.
type recorder struct {
	mu    sync.Mutex
	mine  map[string]bool
	sent  []mail.Message
	calls int
	fail  func(mail.Message) error
}

func (r *recorder) Send(_ context.Context, m mail.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.mine[m.To] {
		return nil
	}
	r.calls++
	if r.fail != nil {
		if err := r.fail(m); err != nil {
			return err
		}
	}
	r.sent = append(r.sent, m)
	return nil
}

func (r *recorder) messagesTo(to string) []mail.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []mail.Message
	for _, m := range r.sent {
		if m.To == to {
			out = append(out, m)
		}
	}
	return out
}

func (r *recorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (f *fx) recorder(sale uuid.UUID) (*recorder, string, string) {
	f.T.Helper()
	customer, advisor := f.emailsOf(sale)
	return &recorder{mine: map[string]bool{customer: true, advisor: true}}, customer, advisor
}

func (f *fx) emailsOf(sale uuid.UUID) (customer, advisor string) {
	f.T.Helper()
	if err := f.Pool.QueryRow(f.Ctx, `SELECT c.email, a.business_email FROM sale_registers s
		JOIN customers c ON c.id = s.customer_id JOIN employees a ON a.id = s.assigned_advisor_id WHERE s.id = $1`, sale).Scan(&customer, &advisor); err != nil {
		f.T.Fatal(err)
	}
	return customer, advisor
}

func (f *fx) dispatcher(sender mail.Sender, tune func(*notifications.DispatcherDeps)) *notifications.Dispatcher {
	deps := notifications.DispatcherDeps{DB: f.Pool, Sender: sender, AppBaseURL: "https://app.test", Logger: f.Logger, Now: f.clock.Now}
	if tune != nil {
		tune(&deps)
	}
	return notifications.NewDispatcher(deps)
}

// syncClock makes sure the test clock is not behind the real time at which rows were queued (the database stamps
// them with its own clock). Tests that move time forward do so after this.
func (f *fx) syncClock() {
	f.clock.mu.Lock()
	if now := time.Now(); f.clock.t.Before(now) {
		f.clock.t = now.Add(time.Second)
	}
	f.clock.mu.Unlock()
}

// drain runs the dispatcher until a run finds nothing more to do.
func (f *fx) drain(d *notifications.Dispatcher) (total notifications.Summary) {
	f.T.Helper()
	f.syncClock()
	for i := 0; i < 50; i++ {
		s, err := d.RunOnce(f.Ctx)
		if err != nil {
			f.T.Fatal(err)
		}
		total.Sent += s.Sent
		total.Skipped += s.Skipped
		total.Failed += s.Failed
		total.Retry += s.Retry
		total.Paused = total.Paused || s.Paused
		if s.Sent+s.Skipped+s.Failed+s.Retry == 0 {
			return total
		}
	}
	f.T.Fatal("the dispatcher never ran out of work")
	return total
}

type queued struct {
	status   string
	attempts int
	lastErr  string
	next     time.Time
	sentAt   *time.Time
}

func (f *fx) row(sale uuid.UUID, kind string) queued {
	f.T.Helper()
	var q queued
	var lastErr *string
	err := f.Pool.QueryRow(f.Ctx, `SELECT status::text, attempt_count, last_error, next_attempt_at, dispatched_at FROM notification_logs
		WHERE sale_id = $1 AND notification_type = $2::text::notification_type`, sale, kind).Scan(&q.status, &q.attempts, &lastErr, &q.next, &q.sentAt)
	if err != nil {
		f.T.Fatalf("notification %s of the sale: %v", kind, err)
	}
	if lastErr != nil {
		q.lastErr = *lastErr
	}
	return q
}

func TestEverySaleNotificationIsSentOnceWithItsContent(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, advisor := f.recorder(s.ID)
	d := f.dispatcher(rec, nil)

	sum := f.drain(d)
	if sum.Sent < 2 {
		t.Fatalf("a sale created by a manager queues one email to the customer and one to the advisor: %+v", sum)
	}
	toCustomer, toAdvisor := rec.messagesTo(customer), rec.messagesTo(advisor)
	if len(toCustomer) != 1 || len(toAdvisor) != 1 {
		t.Fatalf("one email each: customer=%d advisor=%d", len(toCustomer), len(toAdvisor))
	}

	c := toCustomer[0]
	if !strings.Contains(c.Subject, s.Code) || !strings.Contains(c.Body, s.Vehicle.Model) || !strings.Contains(c.Body, s.Advisor.Name) {
		t.Fatalf("the customer email names the sale, the vehicle and the advisor:\n%s\n%s", c.Subject, c.Body)
	}
	if c.ReplyTo != advisor || c.HTMLBody == "" || c.EventID == "" {
		t.Fatalf("a reply reaches the advisor, there is an HTML version and an event id: %+v", c)
	}
	a := toAdvisor[0]
	if a.ReplyTo != "" || !strings.Contains(a.Body, "https://app.test/sales/"+s.ID.String()) || !strings.Contains(a.Subject, s.Code) {
		t.Fatalf("the advisor email links to the sale:\n%s\n%s", a.Subject, a.Body)
	}

	for _, kind := range []string{notifications.SaleCreated, notifications.SaleAssigned} {
		r := f.row(s.ID, kind)
		if r.status != "sent" || r.attempts != 1 || r.sentAt == nil || r.lastErr != "" {
			t.Fatalf("%s must be recorded as sent once: %+v", kind, r)
		}
	}

	// A second run, or a second dispatcher, finds nothing to send: no duplicates.
	f.drain(d)
	f.drain(f.dispatcher(rec, nil))
	if len(rec.messagesTo(customer)) != 1 || len(rec.messagesTo(advisor)) != 1 {
		t.Fatal("a notification must be sent once")
	}
}

func TestMilestoneEmailsGoToTheCustomerWhenTheTaskCompletes(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, _ := f.recorder(s.ID)
	d := f.dispatcher(rec, nil)
	f.drain(d)

	f.advance(s.ID, f.advisor, 2) // completes tasks 1 and 2; only task 1 notifies the customer
	f.drain(d)
	msgs := rec.messagesTo(customer)
	if len(msgs) != 2 {
		t.Fatalf("the creation email and the task 1 milestone email: %d", len(msgs))
	}
	if !strings.Contains(msgs[1].Subject, "cotización") || msgs[1].ReplyTo == "" {
		t.Fatalf("milestone email: %+v", msgs[1])
	}
}

func TestReassigningNotifiesTheCustomerAndBothAdvisors(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, oldAdvisor := f.recorder(s.ID)
	rec.mine[f.emailOfEmployee(f.other.ID)] = true
	f.drain(f.dispatcher(rec, nil))

	if _, err := f.sales.Reassign(f.Ctx, f.manager.Actor(), s.ID, f.other.ID, "Carga de trabajo"); err != nil {
		t.Fatal(err)
	}
	f.drain(f.dispatcher(rec, nil))

	newAdvisor := f.emailOfEmployee(f.other.ID)
	changed := rec.messagesTo(customer)
	if len(changed) != 2 || !strings.Contains(changed[1].Subject, "asesor ha cambiado") || changed[1].ReplyTo != newAdvisor {
		t.Fatalf("the customer is told about the new advisor, and replies reach them: %+v", changed)
	}
	if got := rec.messagesTo(newAdvisor); len(got) != 1 || !strings.Contains(got[0].Subject, "Se le asignó") {
		t.Fatalf("the new advisor is told: %+v", got)
	}
	old := rec.messagesTo(oldAdvisor)
	if len(old) != 2 || !strings.Contains(old[1].Subject, "ya no está asignada") || !strings.Contains(old[1].Body, f.nameOfEmployee(f.other.ID)) {
		t.Fatalf("the previous advisor is told who has the sale now: %+v", old)
	}
}

func (f *fx) nameOfEmployee(id uuid.UUID) string {
	f.T.Helper()
	var name string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT first_name || ' ' || last_name FROM employees WHERE id = $1`, id).Scan(&name); err != nil {
		f.T.Fatal(err)
	}
	return name
}

func (f *fx) emailOfEmployee(id uuid.UUID) string {
	f.T.Helper()
	var email string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT business_email FROM employees WHERE id = $1`, id).Scan(&email); err != nil {
		f.T.Fatal(err)
	}
	return email
}

func TestAnExhaustedQuotaPausesSendingWithoutCountingAttempts(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, _ := f.recorder(s.ID)
	rec.fail = func(mail.Message) error { return mail.ErrQuotaExceeded }
	d := f.dispatcher(rec, nil)

	sum := f.drain(d)
	if !sum.Paused || sum.Sent != 0 {
		t.Fatalf("the run must stop and report the pause: %+v", sum)
	}
	r := f.row(s.ID, notifications.SaleCreated)
	if r.status != "pending" || r.attempts != 0 || !strings.Contains(r.lastErr, "quota") {
		t.Fatalf("the notification waits, without using up an attempt, and says why: %+v", r)
	}
	calls := rec.callCount()
	f.clock.Advance(30 * time.Minute)
	if f.drain(d); rec.callCount() != calls {
		t.Fatal("while sending is paused the email service must not be called")
	}

	rec.fail = nil
	f.clock.Advance(31 * time.Minute) // an hour has passed since the quota ran out
	f.drain(d)
	if len(rec.messagesTo(customer)) != 1 || f.row(s.ID, notifications.SaleCreated).status != "sent" {
		t.Fatal("once the pause is over the pending notification is sent")
	}
}

func TestTransientFailuresAreRetriedWithBackoffAndThenGivenUp(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, _ := f.recorder(s.ID)
	rec.fail = func(m mail.Message) error {
		if m.To == customer {
			return mail.ErrTransient
		}
		return nil
	}
	d := f.dispatcher(rec, func(deps *notifications.DispatcherDeps) {
		deps.MaxAttempts = 3
		deps.Backoff = []time.Duration{time.Minute, 2 * time.Minute}
	})

	f.drain(d)
	r := f.row(s.ID, notifications.SaleCreated)
	if r.status != "pending" || r.attempts != 1 || !r.next.After(f.clock.Now()) {
		t.Fatalf("first failure: pending, one attempt, retry later: %+v", r)
	}
	if got := f.row(s.ID, notifications.SaleAssigned); got.status != "sent" {
		t.Fatalf("a failing email must not hold back the others: %+v", got)
	}

	calls := rec.callCount()
	f.drain(d)
	if rec.callCount() != calls {
		t.Fatal("a notification is not retried before its time")
	}

	f.clock.Advance(time.Minute + time.Second)
	f.drain(d)
	r = f.row(s.ID, notifications.SaleCreated)
	if r.status != "pending" || r.attempts != 2 {
		t.Fatalf("second failure: %+v", r)
	}
	if wait := r.next.Sub(f.clock.Now()); wait < time.Minute || wait > 2*time.Minute+time.Second {
		t.Fatalf("the second wait is the longer one: %v", wait)
	}

	f.clock.Advance(2*time.Minute + time.Second)
	f.drain(d)
	r = f.row(s.ID, notifications.SaleCreated)
	if r.status != "failed" || r.attempts != 3 || r.lastErr == "" {
		t.Fatalf("after the last attempt it is given up and the error is kept: %+v", r)
	}
	calls = rec.callCount()
	f.clock.Advance(24 * time.Hour)
	if f.drain(d); rec.callCount() != calls {
		t.Fatal("a failed notification is never tried again")
	}
}

func TestPermanentRefusalsAreNotRetried(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, advisor := f.recorder(s.ID)
	rec.fail = func(m mail.Message) error {
		if m.To == customer {
			return mail.ErrRejected
		}
		return mail.ErrRecipientNotAllowed
	}
	f.drain(f.dispatcher(rec, nil))

	if r := f.row(s.ID, notifications.SaleCreated); r.status != "failed" || r.attempts != 1 {
		t.Fatalf("an address the provider refuses fails at once: %+v", r)
	}
	r := f.row(s.ID, notifications.SaleAssigned)
	if r.status != "skipped" || r.attempts != 0 || !strings.Contains(r.lastErr, "lista permitida") {
		t.Fatalf("a recipient outside the allow-list is skipped, and it never reached the provider: %+v", r)
	}
	_ = advisor
}

func TestRefusedCredentialsPauseSendingAndKeepNotificationsPending(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, _, _ := f.recorder(s.ID)
	rec.fail = func(mail.Message) error { return mail.ErrUnauthorized }
	d := f.dispatcher(rec, nil)

	if sum := f.drain(d); !sum.Paused {
		t.Fatalf("a configuration problem pauses sending: %+v", sum)
	}
	r := f.row(s.ID, notifications.SaleCreated)
	if r.status != "pending" || r.attempts != 0 {
		t.Fatalf("a configuration problem does not burn the notification's attempts: %+v", r)
	}
	calls := rec.callCount()
	f.drain(d)
	if rec.callCount() != calls {
		t.Fatal("no new calls while paused")
	}
}

func TestNotificationsOfADeletedSaleOrADeactivatedEmployeeAreSkipped(t *testing.T) {
	f := newFx(t)
	deleted := f.newSale(f.advisor)
	rec, _, _ := f.recorder(deleted.ID)
	if err := f.sales.Delete(f.Ctx, f.admin.Actor(), deleted.ID, "Venta de prueba", deleted.Code); err != nil {
		t.Fatal(err)
	}

	kept := f.newSale(f.other)
	rec2, _, otherEmail := f.recorder(kept.ID)
	rec.mine = map[string]bool{}
	for k := range rec2.mine {
		rec.mine[k] = true
	}
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE employees SET is_active = false, deactivated_at = now(), deactivated_by = $2 WHERE id = $1`, f.other.ID, f.admin.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(f.dispatcher(rec, nil))

	var skipped int
	if err := f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM notification_logs WHERE status = 'skipped' AND last_error = 'la venta ya no existe'
		AND recipient_email = ANY($1)`, []string{f.emailOfEmployee(f.advisor.ID)}).Scan(&skipped); err != nil {
		t.Fatal(err)
	}
	if skipped == 0 {
		t.Fatal("notifications of a sale that no longer exists are skipped")
	}
	if r := f.row(kept.ID, notifications.SaleAssigned); r.status != "skipped" || !strings.Contains(r.lastErr, "desactivado") {
		t.Fatalf("an employee who was deactivated is not emailed: %+v", r)
	}
	if len(rec.messagesTo(otherEmail)) != 0 {
		t.Fatal("nothing may be sent to a deactivated employee")
	}
	if r := f.row(kept.ID, notifications.SaleCreated); r.status != "sent" {
		t.Fatalf("the customer of that sale is still emailed: %+v", r)
	}
}

func TestOnlyOneDispatcherSendsEachNotification(t *testing.T) {
	f := newFx(t)
	var sales []uuid.UUID
	rec := &recorder{mine: map[string]bool{}}
	for i := 0; i < 5; i++ {
		s := f.newSale(f.advisor)
		sales = append(sales, s.ID)
		c, a := f.emailsOf(s.ID)
		rec.mine[c], rec.mine[a] = true, true
	}
	f.syncClock()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := f.dispatcher(rec, nil)
			for j := 0; j < 20; j++ {
				if _, err := d.RunOnce(f.Ctx); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]int{}
	rec.mu.Lock()
	for _, m := range rec.sent {
		seen[m.EventID]++
	}
	rec.mu.Unlock()
	if len(seen) < 10 {
		t.Fatalf("five sales queue at least ten emails: %d sent", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("notification %s was sent %d times by competing dispatchers", id, n)
		}
	}
	_ = sales
}

func TestTemplatesEscapeWhatTheyShow(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	if _, err := f.Pool.Exec(f.Ctx, `UPDATE employees SET first_name = $2 WHERE id = $1`, f.advisor.ID, `<b>Ana</b> & Hijos`); err != nil {
		t.Fatal(err)
	}
	rec, customer, _ := f.recorder(s.ID)
	f.drain(f.dispatcher(rec, nil))

	mail := rec.messagesTo(customer)[0]
	if strings.Contains(mail.HTMLBody, "<b>Ana</b>") || !strings.Contains(mail.HTMLBody, "&lt;b&gt;Ana&lt;/b&gt; &amp; Hijos") {
		t.Fatalf("HTML must escape names: %s", mail.HTMLBody)
	}
	if !strings.Contains(mail.Body, "<b>Ana</b> & Hijos") {
		t.Fatal("the plain-text version shows the name as it is")
	}
}

func TestATypeWithoutATemplateFailsInsteadOfBeingLost(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	rec, customer, _ := f.recorder(s.ID)
	if _, err := f.Pool.Exec(f.Ctx, `INSERT INTO notification_logs (notification_type, sale_id, recipient_email, dedupe_key)
		VALUES ('sla_warning', $1, $2, $3)`, s.ID, customer, "test:"+s.ID.String()); err != nil {
		t.Fatal(err)
	}
	f.drain(f.dispatcher(rec, nil))
	if r := f.row(s.ID, "sla_warning"); r.status != "failed" || !strings.Contains(r.lastErr, "plantilla") {
		t.Fatalf("%+v", r)
	}
}
