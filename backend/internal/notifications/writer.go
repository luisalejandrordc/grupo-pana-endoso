package notifications

import (
	"context"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Writer translates business events into transactional notification outbox entries.
//
// Every entry is written inside the caller's transaction. When the Writer has a Waker, the dispatcher is woken
// after that transaction commits, otherwise it only finds new notifications on its periodic look.
type Writer struct {
	Waker *Waker
}

// enqueue writes the notification and arranges for the dispatcher to be woken once q's transaction commits.
func (w Writer) enqueue(ctx context.Context, q execer, n Notification) error {
	if err := Enqueue(ctx, q, n); err != nil {
		return err
	}
	database.AfterCommit(q, w.Waker.Wake)
	return nil
}

type recipient struct {
	ID    uuid.UUID
	Email string
}

// customerOf retrieves the customer's email associated with a sale.
func customerOf(ctx context.Context, q tasks.Querier, saleID uuid.UUID) (recipient, error) {
	var r recipient
	err := q.QueryRow(ctx, `
		SELECT c.id, c.email
		FROM sale_registers s
		JOIN customers c ON c.id = s.customer_id
		WHERE s.id = $1
	`, saleID).Scan(&r.ID, &r.Email)
	return r, err
}

// employeeOf retrieves an employee's notification email.
func employeeOf(ctx context.Context, q tasks.Querier, employeeID uuid.UUID) (recipient, bool, error) {
	var r recipient
	err := q.QueryRow(ctx, `
		SELECT id, business_email
		FROM employees
		WHERE id = $1
	`, employeeID).Scan(&r.ID, &r.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// TaskCompleted queues a customer notification when a milestone task is completed.
func (w Writer) TaskCompleted(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
	taskID uuid.UUID, taskNumber int,
) error {
	kind := MilestoneTaskType(taskNumber)
	if kind == "" {
		return nil
	}
	customer, err := customerOf(ctx, q, saleID)
	if err != nil {
		return err
	}
	return w.enqueue(ctx, q, Notification{
		Type:       kind,
		DedupeKey:  kind + ":" + saleID.String(),
		SaleID:     &saleID,
		TaskID:     &taskID,
		CustomerID: &customer.ID,
		Email:      customer.Email,
	})
}

// SaleCreated queues the customer notification and, when the sale
// was assigned to another advisor, the advisor notification.
func (w Writer) SaleCreated(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
	advisorID uuid.UUID, createdBy uuid.UUID,
) error {
	customer, err := customerOf(ctx, q, saleID)
	if err != nil {
		return err
	}
	if err := w.enqueue(ctx, q, Notification{
		Type:       SaleCreated,
		DedupeKey:  SaleCreated + ":" + saleID.String(),
		SaleID:     &saleID,
		CustomerID: &customer.ID,
		Email:      customer.Email,
	}); err != nil {
		return err
	}
	// An advisor who creates a sale for themselves is not told about it.
	if createdBy == advisorID {
		return nil
	}
	advisor, found, err := employeeOf(ctx, q, advisorID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("assigned advisor %s not found", advisorID)
	}
	return w.enqueue(ctx, q, Notification{
		Type:       SaleAssigned,
		DedupeKey:  SaleAssigned + ":" + saleID.String() + ":" + advisorID.String(),
		SaleID:     &saleID,
		EmployeeID: &advisor.ID,
		Email:      advisor.Email,
	})
}

// AdvisorChanged queues notifications for the customer, the new advisor, and the previous advisor.
func (w Writer) AdvisorChanged(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
	previous uuid.UUID, next uuid.UUID,
) error {
	customer, err := customerOf(ctx, q, saleID)
	if err != nil {
		return err
	}
	// Each change gets its own key, so reassigning back and forth notifies every time.
	stamp := ids.New().String()
	if err := w.enqueue(ctx, q, Notification{
		Type:       AdvisorChanged,
		DedupeKey:  "advisor_changed:" + saleID.String() + ":" + stamp,
		SaleID:     &saleID,
		CustomerID: &customer.ID,
		Email:      customer.Email,
	}); err != nil {
		return err
	}
	// Notify the new advisor.
	advisor, found, err := employeeOf(ctx, q, next)
	if err != nil {
		return err
	}
	if found {
		if err := w.enqueue(ctx, q, Notification{
			Type:       SaleAssigned,
			DedupeKey:  "sale_assigned:" + saleID.String() + ":" + stamp,
			SaleID:     &saleID,
			EmployeeID: &advisor.ID,
			Email:      advisor.Email,
		}); err != nil {
			return err
		}
	}
	// Notify the previous advisor.
	advisor, found, err = employeeOf(ctx, q, previous)
	if err != nil {
		return err
	}
	if found {
		return w.enqueue(ctx, q, Notification{
			Type:       SaleUnassigned,
			DedupeKey:  "sale_unassigned:" + saleID.String() + ":" + stamp,
			SaleID:     &saleID,
			EmployeeID: &advisor.ID,
			Email:      advisor.Email,
		})
	}
	return nil
}

// EmployeeActivation queues the activation email of an employee (also used to resend it). The link is minted
// by the dispatcher when it sends, so each call gets its own dedupe key and a resend always goes out.
func (w Writer) EmployeeActivation(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID, email string) error {
	return w.enqueue(ctx, tx, Notification{
		Type:       EmployeeActivation,
		DedupeKey:  EmployeeActivation + ":" + employeeID.String() + ":" + ids.New().String(),
		EmployeeID: &employeeID,
		Email:      email,
	})
}

// PasswordReset queues a password reset email for an employee.
func (w Writer) PasswordReset(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID, email string) error {
	return w.enqueue(ctx, tx, Notification{
		Type:       EmployeePasswordReset,
		DedupeKey:  EmployeePasswordReset + ":" + employeeID.String() + ":" + ids.New().String(),
		EmployeeID: &employeeID,
		Email:      email,
	})
}

// CustomerActivation queues the welcome and activation email of a customer, the equivalent of EmployeeActivation.
// It is used when the customer is created and again to resend it, so each call gets its own dedupe key.
func (w Writer) CustomerActivation(ctx context.Context, tx pgx.Tx, customerID uuid.UUID, email string) error {
	return w.enqueue(ctx, tx, Notification{
		Type:       CustomerActivation,
		DedupeKey:  CustomerActivation + ":" + customerID.String() + ":" + ids.New().String(),
		CustomerID: &customerID,
		Email:      email,
	})
}

// EmployeeActivated queues the confirmation that an employee's account is now active. An account is activated once,
// so the dedupe key makes a repeated call a no-op.
func (w Writer) EmployeeActivated(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID, email string) error {
	return w.enqueue(ctx, tx, Notification{
		Type: EmployeeActivated, DedupeKey: EmployeeActivated + ":" + employeeID.String(),
		EmployeeID: &employeeID, Email: email,
	})
}

// CustomerActivated queues the confirmation that a customer's account is now active.
func (w Writer) CustomerActivated(ctx context.Context, tx pgx.Tx, customerID uuid.UUID, email string) error {
	return w.enqueue(ctx, tx, Notification{
		Type: CustomerActivated, DedupeKey: CustomerActivated + ":" + customerID.String(),
		CustomerID: &customerID, Email: email,
	})
}

// Compile-time interface checks.
var (
	_ sales.Notifier = Writer{}
	_ tasks.Notifier = Writer{}
)
