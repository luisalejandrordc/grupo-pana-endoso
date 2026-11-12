// Package mail defines how the application sends email and provides the senders available today.
// The Google Apps Script sender will be added here later, behind the same interface.
package mail

import (
	"context"
	"errors"
	"log/slog"
)

// Message is an email. Body is the plain-text version and is required; the HTML version, the reply address and
// the event id are optional. The event id travels to the sender and comes back in its answer, which lets a log
// line tie a delivery to the notification that caused it.
type Message struct {
	To       string
	Subject  string
	Body     string
	HTMLBody string
	ReplyTo  string
	EventID  string
}

// Sender sends emails.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// The ways a send can fail, so callers can decide what to do next without reading error text. Test them with
// errors.Is; the wrapped error carries the detail.
var (
	// ErrQuotaExceeded means the daily sending quota of the provider is used up. Nothing is wrong with the
	// message: try again later, and stop sending until then.
	ErrQuotaExceeded = errors.New("mail: the daily sending quota is exhausted")
	// ErrUnauthorized means the provider refused our signature or timestamp: a configuration problem (wrong
	// secret, or a clock that is off by more than five minutes). Retrying the same message cannot help.
	ErrUnauthorized = errors.New("mail: the email service rejected our credentials")
	// ErrRejected means the provider will never accept this message (an invalid or missing field, or an invalid
	// recipient address). Do not retry it.
	ErrRejected = errors.New("mail: the email service rejected the message")
	// ErrTransient is any other failure (network, timeout, an unexpected answer, a failure inside the
	// provider). A later attempt may succeed.
	ErrTransient = errors.New("mail: the email could not be sent right now")
	// ErrRecipientNotAllowed means the environment only sends to an allow-list and the recipient is not on it.
	ErrRecipientNotAllowed = errors.New("mail: the recipient is not on this environment's allow-list")
)

// ErrNotConfigured is returned by the sender used where no real email service exists yet.
var ErrNotConfigured = errors.New("mail: no email sender is configured for this environment")

type unconfigured struct{}

func (unconfigured) Send(context.Context, Message) error { return ErrNotConfigured }

// Unconfigured returns a sender that always fails, so a missing email service is never silent.
func Unconfigured() Sender { return unconfigured{} }

type logSender struct{ logger *slog.Logger }

// NewLogSender returns a sender that writes the whole email, links included, to the log instead of
// sending it. It is only for local development and tests.
func NewLogSender(logger *slog.Logger) Sender { return logSender{logger: logger} }

func (s logSender) Send(_ context.Context, msg Message) error {
	s.logger.Info("email not sent (local log sender)", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}
