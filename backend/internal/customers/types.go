// Package customers manages the buyers of vehicles: create, edit, activate, deactivate and delete, the
// existence lookup by document, and the Codart name autofill with its quota guard. Admins and Managers
// see every customer; an Advisor only the customers in their scope (see inAdvisorScope).
package customers

import (
	"time"

	"github.com/google/uuid"
)

// Customer types. Diplomático is a person (B2C); the others are organizations (B2B).
const (
	TypeDiplomat         = "diplomatico"
	TypeEmbassy          = "embajada"
	TypeCatholicEntity   = "entidad_religiosa_catolica"
	TypeOtherReligious   = "entidad_religiosa_no_catolica"
	TypeInternationalOrg = "organismo_internacional"
)

// Customer is the customer as the API shows it. Password data never leaves the database.
type Customer struct {
	ID               uuid.UUID `json:"id"`
	CustomerType     string    `json:"customerType"`
	DocumentType     string    `json:"documentType"`
	DocumentNumber   string    `json:"documentNumber"`
	FirstName        *string   `json:"firstName"`
	LastName         *string   `json:"lastName"`
	Gender           *string   `json:"gender"`
	LegalName        *string   `json:"legalName"`
	DisplayName      string    `json:"displayName"`
	NameSource       string    `json:"nameSource"`
	Nationality      string    `json:"nationality"`
	Phone            string    `json:"phone"`
	Email            string    `json:"email"`
	ContactFirstName *string   `json:"contactFirstName"`
	ContactLastName  *string   `json:"contactLastName"`
	ContactJobTitle  *string   `json:"contactJobTitle"`
	PortalStatus     string    `json:"portalStatus"` // pending or active
	IsActive         bool      `json:"isActive"`
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Brief is what an Advisor sees of a customer who is not in their scope: the legal name and nothing else
// that identifies contact data or sales.
type Brief struct {
	ID           uuid.UUID `json:"id"`
	DisplayName  string    `json:"displayName"`
	CustomerType string    `json:"customerType"`
	IsActive     bool      `json:"isActive"`
}

// CreateInput is the body of a create request. Fields that do not apply to the customer type are ignored.
type CreateInput struct {
	CustomerType     string `json:"customerType"`
	DocumentType     string `json:"documentType"`
	DocumentNumber   string `json:"documentNumber"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	Gender           string `json:"gender"`
	LegalName        string `json:"legalName"`
	NameSource       string `json:"nameSource"`
	Nationality      string `json:"nationality"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	ContactFirstName string `json:"contactFirstName"`
	ContactLastName  string `json:"contactLastName"`
	ContactJobTitle  string `json:"contactJobTitle"`
}

// UpdateInput is the body of an update request. Only the fields that are present change. Version is
// required. Editing a name without saying where it came from marks it as manually entered.
type UpdateInput struct {
	Version          *int    `json:"version"`
	CustomerType     *string `json:"customerType"`
	DocumentType     *string `json:"documentType"`
	DocumentNumber   *string `json:"documentNumber"`
	FirstName        *string `json:"firstName"`
	LastName         *string `json:"lastName"`
	Gender           *string `json:"gender"`
	LegalName        *string `json:"legalName"`
	NameSource       *string `json:"nameSource"`
	Nationality      *string `json:"nationality"`
	Phone            *string `json:"phone"`
	Email            *string `json:"email"`
	ContactFirstName *string `json:"contactFirstName"`
	ContactLastName  *string `json:"contactLastName"`
	ContactJobTitle  *string `json:"contactJobTitle"`
}

// ListFilter selects and orders the list of customers.
type ListFilter struct {
	Search       string // name, document number or email prefix
	CustomerType string
	Status       string // active, inactive or all (default all)
	Page         int
	PageSize     int
	Sort         string // displayName (default), -displayName, createdAt, -createdAt
}

// ListResult is one page of customers.
type ListResult struct {
	Items    []Customer `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

// LookupResult answers "does a customer with this document exist?". Access is "full" (Customer is set) or
// "name_only" (Brief is set). It never mentions the customer's sales.
type LookupResult struct {
	Exists   bool      `json:"exists"`
	Access   string    `json:"access,omitempty"`
	Customer *Customer `json:"customer,omitempty"`
	Brief    *Brief    `json:"brief,omitempty"`
}

// IdentityResult is the name data found for a DNI or RUC. Source tells where it came from: "provider"
// (Codart) or "database" (a customer we already have, which costs no provider call).
type IdentityResult struct {
	DocumentType   string  `json:"documentType"`
	DocumentNumber string  `json:"documentNumber"`
	Source         string  `json:"source"`
	FirstName      *string `json:"firstName,omitempty"`
	LastName       *string `json:"lastName,omitempty"`
	LegalName      *string `json:"legalName,omitempty"`
}

// QuotaStatus is the Codart usage in the current month (Lima time).
type QuotaStatus struct {
	Limit       int       `json:"limit"`
	Used        int       `json:"used"`
	Remaining   int       `json:"remaining"`
	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`
}
