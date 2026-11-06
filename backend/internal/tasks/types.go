package tasks

import (
	"time"

	"github.com/google/uuid"
)

// Summary is one task as the sale detail and the task list show it.
type Summary struct {
	Number                   int        `json:"number"`
	Name                     string     `json:"name"`
	MilestoneNumber          int        `json:"milestoneNumber"`
	Status                   Status     `json:"status"`
	StartedAt                *time.Time `json:"startedAt"`
	CompletedAt              *time.Time `json:"completedAt"`
	DueDate                  *time.Time `json:"dueDate"`
	CompletionPhase          *Phase     `json:"completionPhase"`
	RecommendedDurationHours *int       `json:"recommendedDurationHours"`
	IsEligible               bool       `json:"isEligible"`
	Version                  int        `json:"version"`
}

// DocumentFile is the metadata of an uploaded file. The storage key never leaves the server.
type DocumentFile struct {
	ID             uuid.UUID `json:"id"`
	FileName       string    `json:"fileName"`
	MimeType       string    `json:"mimeType"`
	SizeBytes      int       `json:"sizeBytes"`
	UploadedAt     time.Time `json:"uploadedAt"`
	UploadedByName string    `json:"uploadedByName"`
}

// DocumentSlot is one document the task asks for. Required already accounts for the customer type (the power
// of attorney is required unless the customer is a Diplomático).
type DocumentSlot struct {
	SlotCode        string        `json:"slotCode"`
	Label           string        `json:"label"`
	AllowedFormat   string        `json:"allowedFormat"`
	Required        bool          `json:"required"`
	CustomerVisible bool          `json:"customerVisible"`
	File            *DocumentFile `json:"file"`
}

// Permissions tells the interface which actions this person can take on this task right now, so it never has
// to re-derive the rules. The server still checks every request.
type Permissions struct {
	CanStart     bool `json:"canStart"`
	CanComplete  bool `json:"canComplete"`
	CanEditData  bool `json:"canEditData"` // saving fields (and files) while in progress, or correcting when completed
	NeedsReason  bool `json:"needsReason"` // editing a completed task requires a written reason
	CanSaveNotes bool `json:"canSaveNotes"`
	CanRestart   bool `json:"canRestart"`
	CanReopen    bool `json:"canReopen"`
}

// Detail is one task with its data, calculated values, documents and the actions available.
type Detail struct {
	Summary
	SaleID      uuid.UUID      `json:"saleId"`
	Notes       *string        `json:"notes"`
	Data        any            `json:"data"`
	Calculated  map[string]any `json:"calculated"`
	Documents   []DocumentSlot `json:"documents"`
	Warnings    []Warning      `json:"warnings"`
	Permissions Permissions    `json:"permissions"`
}

// SaveInput is the body of a data save. Data is the task-specific object; Reason is required when the task is
// already completed.
type SaveInput struct {
	Version *int
	Reason  string
	Data    []byte
}
