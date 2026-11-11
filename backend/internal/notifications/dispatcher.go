package notifications

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/platform/database"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DispatcherDeps are the dispatcher's dependencies.
type DispatcherDeps struct {
	DB         database.TxStarter
	Sender     mail.Sender
	AppBaseURL string // the frontend address, used in the links of emails to employees
	Logger     *slog.Logger
	Now        func() time.Time // nil means time.Now

	// The settings below have defaults suited to the prototype and are only set by tests.
	MaxAttempts int             // sends of one notification before it is given up as failed (default 5)
	Backoff     []time.Duration // wait before the 2nd, 3rd... attempt (default 5m, 15m, 1h, 3h)
	QuotaPause  time.Duration   // how long to stop sending after the daily quota ran out (default 1h)
	AuthPause   time.Duration   // how long to stop after the email service refused our credentials (default 10m)
	BatchSize   int             // notifications handled per run (default 20)
	SendTimeout time.Duration   // one delivery (default 30s)
}

// Dispatcher sends the notifications that sales and tasks queued in notification_logs.
//
// Each run takes pending rows one at a time with FOR UPDATE SKIP LOCKED, keeps that row locked while it sends
// and records the outcome in the same transaction. Two dispatchers (or two overlapping runs) therefore never
// pick the same notification. The guarantee is "at most once per successful commit": if the process dies after
// the email left but before the commit, the row is still pending and the email is sent again. For this
// prototype a rare duplicate is preferred over a lost email.
type Dispatcher struct {
	db          database.TxStarter
	sender      mail.Sender
	baseURL     string
	logger      *slog.Logger
	now         func() time.Time
	maxAttempts int
	backoff     []time.Duration
	quotaPause  time.Duration
	authPause   time.Duration
	batch       int
	sendTimeout time.Duration

	mu          sync.Mutex
	pausedUntil time.Time
}

// NewDispatcher builds the dispatcher.
func NewDispatcher(d DispatcherDeps) *Dispatcher {
	x := &Dispatcher{
		db: d.DB, sender: d.Sender, baseURL: strings.TrimRight(d.AppBaseURL, "/"), logger: d.Logger, now: d.Now,
		maxAttempts: d.MaxAttempts, backoff: d.Backoff, quotaPause: d.QuotaPause, authPause: d.AuthPause,
		batch: d.BatchSize, sendTimeout: d.SendTimeout,
	}
	if x.now == nil {
		x.now = time.Now
	}
	if x.maxAttempts <= 0 {
		x.maxAttempts = 5
	}
	if len(x.backoff) == 0 {
		x.backoff = []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour}
	}
	if x.quotaPause <= 0 {
		x.quotaPause = time.Hour
	}
	if x.authPause <= 0 {
		x.authPause = 10 * time.Minute
	}
	if x.batch <= 0 {
		x.batch = 20
	}
	if x.sendTimeout <= 0 {
		x.sendTimeout = 30 * time.Second
	}
	if x.logger == nil {
		x.logger = slog.Default()
	}
	return x
}

// Summary is what one run did.
type Summary struct {
	Sent    int
	Skipped int
	Failed  int
	Retry   int // failed this time, will be tried again later
	// Paused is set when the run stopped early (or did not start) because sending is paused.
	Paused bool
}

type step int

const (
	stepIdle step = iota
	stepSent
	stepSkipped
	stepFailed
	stepRetry
	stepQuota
	stepUnauthorized
)

type claimed struct {
	id         uuid.UUID
	kind       string
	email      string
	saleID     *uuid.UUID
	employeeID *uuid.UUID
	customerID *uuid.UUID
	attempts   int
}

// RunOnce handles up to one batch of due notifications. It never returns an error for a notification that
// could not be sent (that is recorded on its row); an error means the database itself failed.
func (d *Dispatcher) RunOnce(ctx context.Context) (Summary, error) {
	var sum Summary
	if until := d.paused(); d.now().Before(until) {
		sum.Paused = true
		return sum, nil
	}
	for i := 0; i < d.batch; i++ {
		if ctx.Err() != nil {
			return sum, nil // shutting down: stop between notifications, never in the middle of one
		}
		result, err := d.next(ctx)
		if err != nil {
			return sum, err
		}
		switch result {
		case stepIdle:
			return sum, nil
		case stepSent:
			sum.Sent++
		case stepSkipped:
			sum.Skipped++
		case stepFailed:
			sum.Failed++
		case stepRetry:
			sum.Retry++
		case stepQuota:
			d.pause(d.quotaPause)
			d.logger.Warn("the daily email quota is exhausted: sending is paused", "resumes_after", d.quotaPause.String())
			sum.Paused = true
			return sum, nil
		case stepUnauthorized:
			d.pause(d.authPause)
			d.logger.Error("the email service rejected our credentials: check MAIL_APPS_SCRIPT_HMAC_SECRET and this server's clock; sending is paused",
				"resumes_after", d.authPause.String())
			sum.Paused = true
			return sum, nil
		}
	}
	return sum, nil
}

// Run calls RunOnce every interval until the context ends. A panic in one run is logged and does not stop it.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		d.safeRun(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) safeRun(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Error("the notification dispatcher panicked; it will try again", "panic", fmt.Sprint(r))
		}
	}()
	sum, err := d.RunOnce(ctx)
	if err != nil && ctx.Err() == nil {
		d.logger.Error("the notification dispatcher could not run", "error", err)
		return
	}
	if sum.Sent+sum.Skipped+sum.Failed+sum.Retry > 0 {
		d.logger.Info("notifications dispatched", "sent", sum.Sent, "skipped", sum.Skipped, "failed", sum.Failed, "retry_later", sum.Retry)
	}
}

func (d *Dispatcher) paused() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.pausedUntil
}

func (d *Dispatcher) pause(for_ time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pausedUntil = d.now().Add(for_)
}

const claimSQL = `SELECT id, notification_type::text, recipient_email, sale_id, recipient_employee_id, recipient_customer_id, attempt_count
	FROM notification_logs WHERE status = 'pending' AND next_attempt_at <= $1
	ORDER BY next_attempt_at, created_at LIMIT 1 FOR UPDATE SKIP LOCKED`

// next claims one due notification and handles it, all in one transaction.
func (d *Dispatcher) next(ctx context.Context) (step, error) {
	// A notification that was claimed is finished even if the application starts shutting down: an email that
	// left but whose outcome was not recorded would be sent again.
	ctx = context.WithoutCancel(ctx)
	result := stepIdle
	var row claimed
	found := false
	err := database.WithTx(ctx, d.db, func(tx pgx.Tx) error {
		now := d.now()
		err := tx.QueryRow(ctx, claimSQL, now).Scan(&row.id, &row.kind, &row.email, &row.saleID, &row.employeeID, &row.customerID, &row.attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		result, err = d.deliver(ctx, tx, row, now)
		return err
	})
	if err != nil && found {
		// Something unexpected failed while handling this row (not the send itself). Count it as a failed
		// attempt in a fresh transaction, so one broken row cannot block the ones behind it forever.
		d.logger.Error("a notification could not be processed", "id", row.id, "type", row.kind, "error", err)
		if recordErr := d.recordFailure(ctx, row, err); recordErr != nil {
			return stepIdle, errors.Join(err, recordErr)
		}
		return stepRetry, nil
	}
	return result, err
}

// deliver renders and sends one claimed notification and records the outcome on its row.
func (d *Dispatcher) deliver(ctx context.Context, tx pgx.Tx, row claimed, now time.Time) (step, error) {
	if reason, err := d.cannotSend(ctx, tx, row); err != nil {
		return stepIdle, err
	} else if reason != "" {
		return stepSkipped, d.finish(ctx, tx, row, "skipped", reason, now)
	}
	data, reason, err := d.load(ctx, tx, row)
	if err != nil {
		return stepIdle, err
	}
	if reason != "" {
		return stepSkipped, d.finish(ctx, tx, row, "skipped", reason, now)
	}
	email, ok := render(data)
	if !ok {
		return stepFailed, d.finish(ctx, tx, row, "failed", "no hay una plantilla para el tipo "+row.kind, now)
	}
	msg := mail.Message{To: row.email, Subject: email.Subject, Body: email.Text, HTMLBody: email.HTML, EventID: row.id.String()}
	if email.ReplyToAdvisor {
		msg.ReplyTo = data.AdvisorEmail
	}

	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.sendTimeout)
	sendErr := d.sender.Send(sendCtx, msg)
	cancel()

	switch {
	case sendErr == nil:
		return stepSent, d.finish(ctx, tx, row, "sent", "", now)
	case errors.Is(sendErr, mail.ErrRecipientNotAllowed):
		return stepSkipped, d.finish(ctx, tx, row, "skipped", "el destinatario no está en la lista permitida de este entorno", now)
	case errors.Is(sendErr, mail.ErrRejected):
		return stepFailed, d.finish(ctx, tx, row, "failed", truncate(sendErr.Error(), 500), now)
	case errors.Is(sendErr, mail.ErrQuotaExceeded):
		// Nothing is wrong with this notification: leave it pending without counting an attempt.
		return stepQuota, d.hold(ctx, tx, row, sendErr, now.Add(d.quotaPause))
	case errors.Is(sendErr, mail.ErrUnauthorized):
		return stepUnauthorized, d.hold(ctx, tx, row, sendErr, now.Add(d.authPause))
	default:
		return d.retryOrFail(ctx, tx, row, sendErr, now)
	}
}

// cannotSend skips notifications to an employee who has been deactivated since the event.
func (d *Dispatcher) cannotSend(ctx context.Context, tx pgx.Tx, row claimed) (string, error) {
	if row.employeeID == nil {
		return "", nil
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT is_active FROM employees WHERE id = $1`, *row.employeeID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "el empleado ya no existe", nil
	}
	if err != nil {
		return "", err
	}
	if !active {
		return "el empleado está desactivado", nil
	}
	return "", nil
}

// load reads what the template needs. A second return value other than "" says why the email is not needed.
func (d *Dispatcher) load(ctx context.Context, tx pgx.Tx, row claimed) (emailData, string, error) {
	data := emailData{Type: row.kind, AppBaseURL: d.baseURL}
	if row.saleID == nil {
		return data, "la venta ya no existe", nil
	}
	var number int64
	var createdAt time.Time
	var model, version, color, advisorFirst, advisorLast string
	err := tx.QueryRow(ctx, `SELECT s.sale_number, s.created_at, s.customer_legal_name_snapshot, s.vehicle_model_name_snapshot,
			s.vehicle_version_name_snapshot, s.vehicle_color_name_snapshot, a.first_name, a.last_name, a.business_email, a.business_phone
		FROM sale_registers s JOIN employees a ON a.id = s.assigned_advisor_id WHERE s.id = $1`, *row.saleID).
		Scan(&number, &createdAt, &data.CustomerName, &model, &version, &color, &advisorFirst, &advisorLast, &data.AdvisorEmail, &data.AdvisorPhone)
	if errors.Is(err, pgx.ErrNoRows) {
		return data, "la venta ya no existe", nil
	}
	if err != nil {
		return data, "", err
	}
	data.SaleID = row.saleID.String()
	data.SaleCode = SaleCode(createdAt, number)
	data.Vehicle = model + " · " + version + " · " + color
	data.AdvisorName = advisorFirst + " " + advisorLast
	data.GreetingName = data.CustomerName
	if row.employeeID != nil {
		if err := tx.QueryRow(ctx, `SELECT first_name FROM employees WHERE id = $1`, *row.employeeID).Scan(&data.GreetingName); err != nil {
			return data, "", err
		}
	}
	return data, "", nil
}

// finish records a final outcome (sent, skipped or failed).
func (d *Dispatcher) finish(ctx context.Context, tx pgx.Tx, row claimed, status, detail string, now time.Time) error {
	var dispatched any
	if status == "sent" {
		dispatched = now
	}
	var lastError any
	if detail != "" {
		lastError = detail
	}
	_, err := tx.Exec(ctx, `UPDATE notification_logs SET status = $2::notification_status, attempt_count = attempt_count + $3,
			last_error = $4, dispatched_at = $5 WHERE id = $1`, row.id, status, attemptsFor(status), lastError, dispatched)
	return err
}

// attemptsFor says whether the outcome used a send attempt: skipped notifications never reached the provider.
func attemptsFor(status string) int {
	if status == "skipped" {
		return 0
	}
	return 1
}

// hold leaves a notification pending, without counting an attempt, until the given time.
func (d *Dispatcher) hold(ctx context.Context, tx pgx.Tx, row claimed, cause error, until time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE notification_logs SET last_error = $2, next_attempt_at = $3 WHERE id = $1`,
		row.id, truncate(cause.Error(), 500), until)
	return err
}

// retryOrFail counts a failed attempt: the notification is tried again later, or given up after the last one.
func (d *Dispatcher) retryOrFail(ctx context.Context, tx pgx.Tx, row claimed, cause error, now time.Time) (step, error) {
	attempt := row.attempts + 1
	if attempt >= d.maxAttempts {
		d.logger.Warn("a notification was given up after its last attempt", "id", row.id, "type", row.kind, "error", cause)
		_, err := tx.Exec(ctx, `UPDATE notification_logs SET status = 'failed', attempt_count = $2, last_error = $3 WHERE id = $1`,
			row.id, attempt, truncate(cause.Error(), 500))
		return stepFailed, err
	}
	wait := d.backoff[min(attempt-1, len(d.backoff)-1)]
	d.logger.Warn("a notification could not be sent; it will be tried again", "id", row.id, "type", row.kind, "attempt", attempt, "retry_in", wait.String(), "error", cause)
	_, err := tx.Exec(ctx, `UPDATE notification_logs SET attempt_count = $2, last_error = $3, next_attempt_at = $4 WHERE id = $1`,
		row.id, attempt, truncate(cause.Error(), 500), now.Add(wait))
	return stepRetry, err
}

// recordFailure counts a failed attempt for a row whose processing broke unexpectedly.
func (d *Dispatcher) recordFailure(ctx context.Context, row claimed, cause error) error {
	return database.WithTx(ctx, d.db, func(tx pgx.Tx) error {
		_, err := d.retryOrFail(ctx, tx, row, cause, d.now())
		return err
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
