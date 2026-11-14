package notifications

import (
	"context"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Writer translates sales and task events into transactional notification outbox entries.
type Writer struct{}

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
func (Writer) TaskCompleted(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
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
	return Enqueue(ctx, q, Notification{
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
func (Writer) SaleCreated(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
	advisorID uuid.UUID, createdBy uuid.UUID,
) error {
	customer, err := customerOf(ctx, q, saleID)
	if err != nil {
		return err
	}
	if err := Enqueue(ctx, q, Notification{
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
	return Enqueue(ctx, q, Notification{
		Type:       SaleAssigned,
		DedupeKey:  SaleAssigned + ":" + saleID.String() + ":" + advisorID.String(),
		SaleID:     &saleID,
		EmployeeID: &advisor.ID,
		Email:      advisor.Email,
	})
}

// AdvisorChanged queues notifications for the customer,
// the new advisor, and the previous advisor.
func (Writer) AdvisorChanged(ctx context.Context, q tasks.Querier, saleID uuid.UUID,
	previous uuid.UUID, next uuid.UUID,
) error {
	customer, err := customerOf(ctx, q, saleID)
	if err != nil {
		return err
	}
	// Each change gets its own key, so reassigning back and forth notifies every time.
	stamp := ids.New().String()
	if err := Enqueue(ctx, q, Notification{
		Type:       CustomerAdvisorChanged,
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
		if err := Enqueue(ctx, q, Notification{
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
		return Enqueue(ctx, q, Notification{
			Type:       SaleUnassigned,
			DedupeKey:  "sale_unassigned:" + saleID.String() + ":" + stamp,
			SaleID:     &saleID,
			EmployeeID: &advisor.ID,
			Email:      advisor.Email,
		})
	}
	return nil
}

// Compile-time interface checks.
var (
	_ sales.Notifier = Writer{}
	_ tasks.Notifier = Writer{}
)
