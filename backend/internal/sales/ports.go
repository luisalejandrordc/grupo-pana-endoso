package sales

import (
	"context"
	"endosoflow/internal/tasks"

	"github.com/google/uuid"
)

// Notifier receives sale events that produce notifications.
type Notifier interface {
	SaleCreated(
		ctx context.Context,
		q tasks.Querier,
		saleID uuid.UUID,
		advisorID uuid.UUID,
		createdBy uuid.UUID,
	) error

	AdvisorChanged(
		ctx context.Context,
		q tasks.Querier,
		saleID uuid.UUID,
		previous uuid.UUID,
		next uuid.UUID,
	) error
}
