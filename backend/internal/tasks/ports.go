package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SaleRef is what the task engine needs to know about the sale a task belongs to. The sales module owns the
// sale and fills it in; the series, VIN and plate are the identifiers that tasks 3 and 10 write to the sale.
type SaleRef struct {
	ID              uuid.UUID
	State           string
	AdvisorID       uuid.UUID
	CustomerType    string
	CreatedAt       time.Time
	CancelledAtTask *int
	Series          *string
	VIN             *string
	Plate           *string
}

// Identifier names a vehicle identifier stored on the sale (task 3 sets series and VIN, task 10 the plate).
type Identifier string

// The identifiers a task may write to the sale.
const (
	IdentSeries Identifier = "series"
	IdentVIN    Identifier = "vin"
	IdentPlate  Identifier = "plate"
)

// SaleGate is how the task engine reaches the sale. The sales module implements it and main wires it in:
// sales already depends on tasks (it creates the 13 tasks and shows them in the sale detail), so an import in
// the other direction would be a cycle. It also keeps every query on sale_registers inside the sales module.
type SaleGate interface {
	// Find returns the sale, or (nil, nil) when it does not exist. With forUpdate the row is locked, which
	// serializes every change to one sale.
	Find(ctx context.Context, q Querier, id uuid.UUID, forUpdate bool) (*SaleRef, error)
	// SetIdentifiers writes the given identifiers (a nil value clears one). Unique violations come back as
	// database errors that the caller maps with apperr.FromError.
	SetIdentifiers(ctx context.Context, q Querier, id uuid.UUID, values map[Identifier]*string, actor uuid.UUID, now time.Time) error
	// Touch records who changed the sale last, without changing its version (the version guards the sale's
	// own notes, not the progress of its tasks).
	Touch(ctx context.Context, q Querier, id, actor uuid.UUID, now time.Time) error
	// MarkCompleted moves an active sale to completed (task 13 was completed).
	MarkCompleted(ctx context.Context, q Querier, id, actor uuid.UUID, now time.Time) error
	// MarkReactivated moves a completed sale back to active (task 13 was reopened).
	MarkReactivated(ctx context.Context, q Querier, id, actor uuid.UUID, now time.Time) error
}

// DocumentSource lists the document slots of a task with their current files. The documents module
// implements it, so the task engine never reads the documents table itself.
type DocumentSource interface {
	TaskSlots(ctx context.Context, q Querier, taskInstanceID uuid.UUID, taskNumber int, customerType string) ([]DocumentSlot, error)
}
