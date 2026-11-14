// Package sales manages the life of a sale: creating it, listing and reading it, its notes, reassigning its
// advisor, cancelling and reopening it, and deleting a test sale. The 13 tasks inside a sale belong to the
// tasks package.
package sales

import (
	"endosoflow/internal/tasks"
	"time"

	"github.com/google/uuid"
)

// CustomerBrief is the customer as recorded when the sale was created.
type CustomerBrief struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	DocumentType   string    `json:"documentType"`
	DocumentNumber string    `json:"documentNumber"`
	CustomerType   string    `json:"customerType"`
}

// Vehicle is the vehicle as recorded when the sale was created, plus the series, VIN and plate that tasks 3
// and 10 add. The image is the catalog's Main view of the color, read live.
type Vehicle struct {
	ModelID        uuid.UUID `json:"modelId"`
	VersionID      uuid.UUID `json:"versionId"`
	ColorID        uuid.UUID `json:"colorId"`
	Model          string    `json:"model"`
	Version        string    `json:"version"`
	Color          string    `json:"color"`
	ImageURL       *string   `json:"imageUrl"`
	ReferencePrice string    `json:"referencePrice"`
	Series         *string   `json:"series"`
	VIN            *string   `json:"vin"`
	LicensePlate   *string   `json:"licensePlate"`
}

// Advisor is the employee in charge of the sale.
type Advisor struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Phone string    `json:"phone"`
}

// Milestone is one of the six derived stages.
type Milestone struct {
	Number      int                  `json:"number"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	State       tasks.MilestoneState `json:"state"`
}

// CurrentTask is the task the sale is on now.
type CurrentTask struct {
	Number    int          `json:"number"`
	Name      string       `json:"name"`
	Status    tasks.Status `json:"status"`
	DueDate   *time.Time   `json:"dueDate"`
	IsOverdue bool         `json:"isOverdue"`
	Milestone int          `json:"milestoneNumber"`
}

// Cancellation explains why and when a sale was cancelled.
type Cancellation struct {
	Category        string    `json:"category"`
	Text            string    `json:"text"`
	CancelledAt     time.Time `json:"cancelledAt"`
	CancelledByName string    `json:"cancelledByName"`
	AtTaskNumber    int       `json:"atTaskNumber"`
}

// Permissions says what this person can do with this sale right now. The server still checks every request.
type Permissions struct {
	CanEditNotes bool `json:"canEditNotes"`
	CanCancel    bool `json:"canCancel"`
	CanReopen    bool `json:"canReopen"`
	CanReassign  bool `json:"canReassign"`
	CanDelete    bool `json:"canDelete"`
}

// Sale is the full sale detail.
type Sale struct {
	ID               uuid.UUID       `json:"id"`
	Code             string          `json:"code"`
	Customer         CustomerBrief   `json:"customer"`
	Vehicle          Vehicle         `json:"vehicle"`
	Advisor          Advisor         `json:"advisor"`
	Temperature      string          `json:"temperature"`
	State            string          `json:"state"`
	Notes            *string         `json:"notes"`
	QuotedFinalPrice *string         `json:"quotedFinalPrice"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	CompletedAt      *time.Time      `json:"completedAt"`
	Cancellation     *Cancellation   `json:"cancellation"`
	CurrentTask      *CurrentTask    `json:"currentTask"`
	Milestones       []Milestone     `json:"milestones"`
	Tasks            []tasks.Summary `json:"tasks"`
	Permissions      Permissions     `json:"permissions"`
	Version          int             `json:"version"`
}

// ListItem is one row of the sales workspace.
type ListItem struct {
	ID               uuid.UUID    `json:"id"`
	Code             string       `json:"code"`
	CustomerName     string       `json:"customerName"`
	CustomerDocument string       `json:"customerDocument"`
	CustomerType     string       `json:"customerType"`
	VehicleModel     string       `json:"vehicleModel"`
	VehicleVersion   string       `json:"vehicleVersion"`
	VehicleColor     string       `json:"vehicleColor"`
	ImageURL         *string      `json:"imageUrl"`
	AdvisorID        uuid.UUID    `json:"advisorId"`
	AdvisorName      string       `json:"advisorName"`
	Temperature      string       `json:"temperature"`
	State            string       `json:"state"`
	CurrentTask      *CurrentTask `json:"currentTask"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

// List is one page of sales.
type List struct {
	Items    []ListItem `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

// ListFilter selects and orders the sales workspace.
type ListFilter struct {
	Search       string
	State        string
	CurrentTask  int
	Milestone    int
	AdvisorID    *uuid.UUID
	CustomerType string
	Temperature  string
	ModelID      *uuid.UUID
	VersionID    *uuid.UUID
	Page         int
	PageSize     int
	Sort         string // -createdAt (default), createdAt, customerName, -customerName, updatedAt, -updatedAt, dueDate
}

// CreateInput is the body of a create request. AdvisorID is required for a Manager or Admin and must be
// absent (or the caller's own) for an Advisor.
type CreateInput struct {
	CustomerID  uuid.UUID  `json:"customerId"`
	VersionID   uuid.UUID  `json:"versionId"`
	ColorID     uuid.UUID  `json:"colorId"`
	Temperature string     `json:"temperature"`
	Notes       string     `json:"notes"`
	AdvisorID   *uuid.UUID `json:"advisorId"`
}

// HistoryEvent is one entry of a sale's timeline.
type HistoryEvent struct {
	ID         uuid.UUID      `json:"id"`
	At         time.Time      `json:"at"`
	ActorName  string         `json:"actorName"`
	IsSystem   bool           `json:"isSystem"`
	EntityType string         `json:"entityType"`
	TaskNumber *int           `json:"taskNumber"`
	Action     string         `json:"action"`
	Reason     *string        `json:"reason"`
	Changes    map[string]any `json:"changes"`
}

// History is one page of a sale's timeline, newest first.
type History struct {
	Items    []HistoryEvent `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Total    int            `json:"total"`
}
