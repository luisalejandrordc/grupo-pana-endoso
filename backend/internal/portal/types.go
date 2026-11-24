package portal

import (
	"endosoflow/internal/catalog"
	"endosoflow/internal/documents"
	"endosoflow/internal/tasks"
	"time"

	"github.com/google/uuid"
)

// Milestone is one of the six stages of a purchase, with its state: pending, in_progress or completed.
type Milestone struct {
	Number      int    `json:"number"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
}

// Advisor is the contact of the person in charge of the purchase. Only business contact data is shared.
type Advisor struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Gender string `json:"gender"` // M or F, so the screen can say "asesor" or "asesora"
}

// VehicleSummary is the vehicle on a purchase card.
type VehicleSummary struct {
	Model        string `json:"model"`
	Version      string `json:"version"`
	Color        string `json:"color"`
	MainImageURL string `json:"mainImageUrl"`
}

// SaleSummary is one purchase in the customer's list.
type SaleSummary struct {
	ID         uuid.UUID      `json:"id"`
	Code       string         `json:"code"`
	State      string         `json:"state"` // active, completed or cancelled (never why)
	CreatedAt  time.Time      `json:"createdAt"`
	Vehicle    VehicleSummary `json:"vehicle"`
	Milestones []Milestone    `json:"milestones"`
}

// SaleList is the customer's purchases, newest first.
type SaleList struct {
	Items []SaleSummary `json:"items"`
}

// Vehicle is the vehicle of one purchase in full.
type Vehicle struct {
	Model          string                      `json:"model"`
	Version        string                      `json:"version"`
	Color          string                      `json:"color"`
	ReferencePrice string                      `json:"referencePrice"` // a decimal string in USD, as it was when the sale was made
	Categories     []string                    `json:"categories"`
	MainImageURL   string                      `json:"mainImageUrl"`
	Images         []catalog.PresentationImage `json:"images"`
}

// Section is what the customer reads about one completed task: its visible fields and files.
type Section struct {
	TaskNumber      int                          `json:"taskNumber"`
	MilestoneNumber int                          `json:"milestoneNumber"`
	Title           string                       `json:"title"`
	CompletedAt     time.Time                    `json:"completedAt"`
	Fields          []tasks.CustomerField        `json:"fields"`
	Documents       []documents.CustomerDocument `json:"documents"`
}

// SaleDetail is one purchase in full. Sections appear only for completed tasks, so a task that is reopened or
// restarted disappears from here and its milestone moves back, honestly.
type SaleDetail struct {
	ID          uuid.UUID   `json:"id"`
	Code        string      `json:"code"`
	State       string      `json:"state"`
	CreatedAt   time.Time   `json:"createdAt"`
	CompletedAt *time.Time  `json:"completedAt"`
	Vehicle     Vehicle     `json:"vehicle"`
	Advisor     Advisor     `json:"advisor"`
	Milestones  []Milestone `json:"milestones"`
	Sections    []Section   `json:"sections"`
}
