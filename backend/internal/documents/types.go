// Package documents keeps the files attached to the tasks of a sale: upload and replacement with validation
// by content, removal, the list of slots with their current files, short-lived signed downloads, and the
// check that tells the task engine which required files are missing. Files live in private storage and are
// reachable only through a signed address issued after the caller's role and scope are checked.
package documents

import (
	"time"

	"github.com/google/uuid"
)

// MaxBytes is the largest file a slot accepts (10 MB).
const MaxBytes = 10 << 20

// Formats a slot can require, as stored in document_slots.allowed_format.
const (
	FormatPDF   = "pdf"
	FormatExcel = "excel"
)

// File is the current file of a slot.
type File struct {
	ID           uuid.UUID `json:"id"`
	FileName     string    `json:"fileName"`
	MimeType     string    `json:"mimeType"`
	SizeBytes    int       `json:"sizeBytes"`
	ChecksumSHA  string    `json:"checksumSha256"`
	UploadedByID uuid.UUID `json:"uploadedById"`
	UploadedBy   string    `json:"uploadedBy"`
	UploadedAt   time.Time `json:"uploadedAt"`
}

// Slot is one place where a file can be uploaded, with its current file.
type Slot struct {
	SlotCode        string `json:"slotCode"`
	TaskNumber      int    `json:"taskNumber"`
	Label           string `json:"label"`
	AllowedFormat   string `json:"allowedFormat"`
	CustomerVisible bool   `json:"customerVisible"`
	Required        bool   `json:"required"` // needed to complete the task, for this sale's customer type
	File            *File  `json:"file"`
}

// SlotList is the answer to listing the documents of a sale.
type SlotList struct {
	Items []Slot `json:"items"`
}

// UploadInput is an upload or replacement. Reason is required only when the task is already completed.
type UploadInput struct {
	SaleID   uuid.UUID
	Number   int
	Slot     string
	FileName string
	Data     []byte
	Reason   string
}

// DownloadURL is a short-lived address for one file.
type DownloadURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
	FileName  string    `json:"fileName"`
	MimeType  string    `json:"mimeType"`
}
