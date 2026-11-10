// Package outbox queues notifications. Rows are written in the same transaction as the business change, so a
// notification exists if and only if the change committed; a dispatcher (a later sub-phase) sends them. The
// dedupe key makes queueing idempotent: the same event queued twice sends once.
package notifications

import (
	"context"
	"endosoflow/internal/platform/ids"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Notification types accepted by the notification_type enum that sales and tasks produce.
const (
	SaleCreated            = "sale_created"
	SaleAssigned           = "sale_assigned"
	SaleUnassigned         = "sale_unassigned"
	CustomerAdvisorChanged = "customer_advisor_changed"
)

// MilestoneTaskType returns the notification type sent when a milestone task completes, or "" for the tasks
// that do not notify the customer (2, 4, 5, 8, 9, 12).
func MilestoneTaskType(task int) string {
	switch task {
	case 1, 3, 6, 7, 10, 11, 13:
		return "milestone_task_" + itoa(task)
	}
	return ""
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CustomerContact is who a customer notification of a sale goes to.
type CustomerContact struct {
	ID    uuid.UUID
	Email string
}

// LoadCustomerContact returns the customer of a sale and their e-mail. Resolving recipients is the one place
// where this module reads other modules' tables, because the outbox must know whom to write to.
func LoadCustomerContact(ctx context.Context, q queryRower, saleID uuid.UUID) (CustomerContact, error) {
	var c CustomerContact
	err := q.QueryRow(ctx, `SELECT c.id, c.email FROM sale_registers s JOIN customers c ON c.id = s.customer_id WHERE s.id = $1`, saleID).Scan(&c.ID, &c.Email)
	return c, err
}

// Notification is one message to queue. Exactly one of EmployeeID and CustomerID may be set.
type Notification struct {
	Type       string
	DedupeKey  string
	SaleID     *uuid.UUID
	TaskID     *uuid.UUID
	EmployeeID *uuid.UUID
	CustomerID *uuid.UUID
	Email      string
}

// Enqueue writes the notification unless one with the same dedupe key already exists.
func Enqueue(ctx context.Context, q execer, n Notification) error {
	if n.Email == "" {
		return nil // nobody to notify; not an error
	}
	_, err := q.Exec(ctx, `INSERT INTO notification_logs
		(id, notification_type, sale_id, task_instance_id, recipient_employee_id, recipient_customer_id, recipient_email, dedupe_key)
		VALUES ($1, $2::text::notification_type, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (dedupe_key) DO NOTHING`,
		ids.New(), n.Type, n.SaleID, n.TaskID, n.EmployeeID, n.CustomerID, n.Email, n.DedupeKey)
	return err
}
