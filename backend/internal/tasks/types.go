// Package tasks runs the 13-task process of a sale: the task state machine (start, complete, restart,
// reopen), the data each task collects and the rules that must pass before a task can be completed.
// The sale itself belongs to the sales module, which creates the tasks and is reached through SaleGate.
package tasks

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Task and sale states.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"

	SaleActive    = "active"
	SaleCompleted = "completed"
	SaleCancelled = "cancelled"

	// LastTask is the number of the final task of every sale.
	LastTask = 13
)

// Task is one of the 13 steps of a sale, without its data.
type Task struct {
	ID              uuid.UUID  `json:"id"`
	Number          int        `json:"number"`
	Name            string     `json:"name"`
	MilestoneNumber int        `json:"milestoneNumber"`
	Status          string     `json:"status"`
	Eligible        bool       `json:"eligible"` // pending, and the previous task is completed
	StartedAt       *time.Time `json:"startedAt"`
	CompletedAt     *time.Time `json:"completedAt"`
	DueDate         *time.Time `json:"dueDate"`
	CompletionPhase *string    `json:"completionPhase"`
	DurationHours   *int       `json:"durationHours"`
	Notes           *string    `json:"notes"`
	Version         int        `json:"version"`
}

// Detail is a task with the data it collects. Data is a different object for each task number.
type Detail struct {
	Task
	Data              any      `json:"data"`
	RequiredDocuments []string `json:"requiredDocuments"` // document slot codes this task needs to be completed
}

// Warning is a problem that does not stop a save (for example a date earlier than the sale).
type Warning struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SaveResult is the answer to a data save.
type SaveResult struct {
	Task     *Detail   `json:"task"`
	Warnings []Warning `json:"warnings"`
}

// SaveInput is the body of a data save. Only the fields present in Data change; null clears a field. Reason
// is required (and used) only when the task is already completed.
type SaveInput struct {
	Version *int            `json:"version"`
	Reason  string          `json:"reason"`
	Data    json.RawMessage `json:"data"`
}

// NotesInput is the body of a notes save. Notes are free text; an empty value clears them.
type NotesInput struct {
	Notes string `json:"notes"`
}

// ReasonInput is the body of restart and reopen.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// Field is one value of a partial update. Set tells apart a field that was sent from one that was left
// out, and a nil Value means the client sent null (clear the field).
type Field[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON records that the field was present.
func (f *Field[T]) UnmarshalJSON(b []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		f.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.Value = &v
	return nil
}

// Amount is money as typed: a JSON number or string. It is parsed during validation, so a bad value
// becomes a field error instead of a malformed-body error.
type Amount string

// UnmarshalJSON keeps the text of the value.
func (a *Amount) UnmarshalJSON(b []byte) error {
	*a = Amount(strings.Trim(strings.TrimSpace(string(b)), `"`))
	return nil
}
