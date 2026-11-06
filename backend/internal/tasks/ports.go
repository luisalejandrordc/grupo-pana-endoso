package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is satisfied by the pool and by a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SaleRef is what the task engine needs to know about the sale a task belongs to.
type SaleRef struct {
	ID                uuid.UUID
	State             string
	AssignedAdvisorID uuid.UUID
	CustomerType      string
	CreatedAt         time.Time
	Series            *string
	VIN               *string
	Plate             *string
}

// Identifier names a vehicle identifier stored on the sale (Task 3 sets series and VIN, Task 10 the plate).
type Identifier string

// The identifiers a task may write to the sale.
const (
	IdentSeries Identifier = "series"
	IdentVIN    Identifier = "vin"
	IdentPlate  Identifier = "plate"
)

// SaleGate is how the task engine reaches the sale. The sales module implements it and main wires it in,
// because sales already depends on tasks and an import the other way would be a cycle.
type SaleGate interface {
	// Find returns the sale, or (nil, nil) when it does not exist. With forUpdate the row is locked.
	Find(ctx context.Context, q Querier, id uuid.UUID, forUpdate bool) (*SaleRef, error)
	// SetIdentifier writes the series, VIN or plate (nil clears it). Unique violations come back as
	// database errors that the caller maps with apperr.FromError.
	SetIdentifier(ctx context.Context, q Querier, id uuid.UUID, field Identifier, value *string, actor uuid.UUID, now time.Time) error
	// MarkCompleted moves an active sale to completed (Task 13 was completed).
	MarkCompleted(ctx context.Context, q Querier, id uuid.UUID, actor uuid.UUID, now time.Time) error
	// MarkReactivated moves a completed sale back to active (Task 13 was reopened).
	MarkReactivated(ctx context.Context, q Querier, id uuid.UUID, actor uuid.UUID, now time.Time) error
}

// DocumentChecker answers which of the required document slots of a task have no file yet. The documents
// module implements it in phase 1F.
type DocumentChecker interface {
	MissingSlots(ctx context.Context, q Querier, taskInstanceID uuid.UUID, required []string) ([]string, error)
}

// StubDocuments reports that nothing is missing. It exists only so the engine can be exercised before
// the documents module exists; main wires it in local and test environments and nowhere else.
type StubDocuments struct{}

// MissingSlots never reports a missing document.
func (StubDocuments) MissingSlots(context.Context, Querier, uuid.UUID, []string) ([]string, error) {
	return nil, nil
}

// Notifier is told about events that will produce customer notifications. Phase 2D replaces the no-op
// with the outbox writer; until then nothing is queued, so no stale emails can be sent later.
type Notifier interface {
	TaskCompleted(ctx context.Context, q Querier, saleID uuid.UUID, taskNumber int, now time.Time) error
}

// NoopNotifier ignores every event.
type NoopNotifier struct{}

// TaskCompleted does nothing.
func (NoopNotifier) TaskCompleted(context.Context, Querier, uuid.UUID, int, time.Time) error {
	return nil
}
