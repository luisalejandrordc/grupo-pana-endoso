// Package sales manages sale registers: creation with immutable snapshots, the workspace list, detail with
// progress and milestones, notes, reassignment, cancellation and reopening, and the sale history. The 13
// tasks of a sale belong to the tasks module. Admins and Managers see every sale; an Advisor only the sales
// currently assigned to them.
package sales

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Sale states, temperatures and cancellation categories.
const (
	StateActive    = "active"
	StateCompleted = "completed"
	StateCancelled = "cancelled"
)

var (
	temperatures           = []string{"hot", "warm", "cold"}
	cancellationCategories = []string{
		"customer_withdrew", "payment_or_financing_failed", "unit_unavailable",
		"due_diligence_finding", "registration_error", "other",
	}
)

// Sale is a sale register as the API shows it. The customer and vehicle fields are the snapshots taken when
// the sale was created.
type Sale struct {
	ID                   uuid.UUID  `json:"id"`
	CustomerID           uuid.UUID  `json:"customerId"`
	CustomerName         string     `json:"customerName"`
	CustomerDocumentType string     `json:"customerDocumentType"`
	CustomerDocument     string     `json:"customerDocumentNumber"`
	CustomerType         string     `json:"customerType"`
	ModelID              uuid.UUID  `json:"modelId"`
	VersionID            uuid.UUID  `json:"versionId"`
	ColorID              uuid.UUID  `json:"colorId"`
	ModelName            string     `json:"modelName"`
	VersionName          string     `json:"versionName"`
	ColorName            string     `json:"colorName"`
	ReferencePrice       string     `json:"referencePrice"`
	Temperature          string     `json:"temperature"`
	State                string     `json:"state"`
	VehicleSeries        *string    `json:"vehicleSeries"`
	Vin                  *string    `json:"vin"`
	LicensePlate         *string    `json:"licensePlate"`
	AdvisorID            uuid.UUID  `json:"advisorId"`
	AdvisorName          string     `json:"advisorName"`
	Notes                *string    `json:"notes"`
	CurrentTaskNumber    *int       `json:"currentTaskNumber"` // nil when all 13 tasks are completed
	CurrentTaskStatus    *string    `json:"currentTaskStatus"`
	CompletedTasks       int        `json:"completedTasks"`
	CompletedAt          *time.Time `json:"completedAt"`
	CancelledAt          *time.Time `json:"cancelledAt"`
	CancelledAtTask      *int       `json:"cancelledAtTaskNumber"`
	CancellationCategory *string    `json:"cancellationCategory"`
	CancellationText     *string    `json:"cancellationText"`
	Version              int        `json:"version"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// Milestone is the derived state of one of the six milestones.
type Milestone struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	State  string `json:"state"` // pending, in_progress or completed
}

// Detail is a sale with its milestones.
type Detail struct {
	Sale
	Milestones []Milestone `json:"milestones"`
}

// CreateInput is the body of a create request. The model comes from the version.
type CreateInput struct {
	CustomerID        uuid.UUID  `json:"customerId"`
	VersionID         uuid.UUID  `json:"versionId"`
	ColorID           uuid.UUID  `json:"colorId"`
	Temperature       string     `json:"temperature"`
	AssignedAdvisorID *uuid.UUID `json:"assignedAdvisorId"` // an Advisor may omit it or send their own id
	Notes             string     `json:"notes"`
}

// UpdateInput edits the notes of a sale. Version is required.
type UpdateInput struct {
	Version *int    `json:"version"`
	Notes   *string `json:"notes"`
}

// ReassignInput names the new advisor.
type ReassignInput struct {
	AdvisorID uuid.UUID `json:"advisorId"`
}

// CancelInput is the body of a cancellation. Text is required when the category is "other".
type CancelInput struct {
	Category string `json:"category"`
	Text     string `json:"text"`
}

// ReasonInput is the body of a sale reopen.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// ListFilter selects and orders the workspace list.
type ListFilter struct {
	Search       string // customer name, series, VIN or plate; or the start of the customer's document
	State        string
	CurrentTask  int // 1 to 13
	Milestone    int // 1 to 6: sales whose milestone is in progress
	AdvisorID    *uuid.UUID
	CustomerType string
	Temperature  string
	ModelID      *uuid.UUID
	VersionID    *uuid.UUID
	Page         int
	PageSize     int
	Sort         string // createdAt (default is -createdAt), -createdAt, customerName, -customerName
}

// ListResult is one page of sales.
type ListResult struct {
	Items    []Sale `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"`
}

// HistoryEntry is one event of the sale's audit timeline.
type HistoryEntry struct {
	ID         uuid.UUID       `json:"id"`
	ActorID    *uuid.UUID      `json:"actorId"`
	ActorName  *string         `json:"actorName"`
	ActorKind  string          `json:"actorKind"` // employee or system
	EntityType string          `json:"entityType"`
	EntityID   uuid.UUID       `json:"entityId"`
	Action     string          `json:"action"`
	Changes    json.RawMessage `json:"changes"`
	Reason     *string         `json:"reason"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// HistoryResult is one page of history, newest first.
type HistoryResult struct {
	Items    []HistoryEntry `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Total    int            `json:"total"`
}
