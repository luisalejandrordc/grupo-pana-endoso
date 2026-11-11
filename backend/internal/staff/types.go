// Package staff manages employee accounts: create, edit, deactivate, reactivate, delete and the
// advisor picker. Only Admins manage staff; Managers may read the list of active advisors.
package staff

import (
	"time"

	"github.com/google/uuid"
)

// Employee is the employee as the API shows it. Password data never leaves the database.
type Employee struct {
	ID             uuid.UUID  `json:"id"`
	DocumentType   string     `json:"documentType"`
	DocumentNumber string     `json:"documentNumber"`
	FirstName      string     `json:"firstName"`
	LastName       string     `json:"lastName"`
	NameSource     string     `json:"nameSource"`
	Nationality    string     `json:"nationality"`
	Gender         string     `json:"gender"`
	Birthdate      string     `json:"birthdate"`
	HireDate       string     `json:"hireDate"`
	PersonalPhone  *string    `json:"personalPhone"`
	PersonalEmail  *string    `json:"personalEmail"`
	BusinessPhone  string     `json:"businessPhone"`
	BusinessEmail  string     `json:"businessEmail"`
	Role           string     `json:"role"`
	IsActive       bool       `json:"isActive"`
	HasPhoto       bool       `json:"hasPhoto"`
	ActivatedAt    *time.Time `json:"activatedAt"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// Advisor is the short form used by pickers (for example when reassigning a sale).
type Advisor struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
}

// CreateInput is the body of a create request.
type CreateInput struct {
	DocumentType   string  `json:"documentType"`
	DocumentNumber string  `json:"documentNumber"`
	FirstName      string  `json:"firstName"`
	LastName       string  `json:"lastName"`
	NameSource     string  `json:"nameSource"`
	Nationality    string  `json:"nationality"`
	Gender         string  `json:"gender"`
	Birthdate      string  `json:"birthdate"`
	HireDate       string  `json:"hireDate"`
	PersonalPhone  *string `json:"personalPhone"`
	PersonalEmail  *string `json:"personalEmail"`
	BusinessPhone  string  `json:"businessPhone"`
	BusinessEmail  string  `json:"businessEmail"`
	Role           string  `json:"role"`
}

// UpdateInput is the body of an update request. Only the fields that are present change. For the two
// optional personal fields an empty string clears the value. Version is required.
type UpdateInput struct {
	Version        *int    `json:"version"`
	DocumentType   *string `json:"documentType"`
	DocumentNumber *string `json:"documentNumber"`
	FirstName      *string `json:"firstName"`
	LastName       *string `json:"lastName"`
	NameSource     *string `json:"nameSource"`
	Nationality    *string `json:"nationality"`
	Gender         *string `json:"gender"`
	Birthdate      *string `json:"birthdate"`
	HireDate       *string `json:"hireDate"`
	PersonalPhone  *string `json:"personalPhone"`
	PersonalEmail  *string `json:"personalEmail"`
	BusinessPhone  *string `json:"businessPhone"`
	BusinessEmail  *string `json:"businessEmail"`
	Role           *string `json:"role"`
}

// ListFilter selects and orders the list of employees.
type ListFilter struct {
	Search   string // name, document number or email prefix
	Role     string
	Status   string // active, inactive or all (default all)
	Page     int
	PageSize int
	Sort     string // lastName (default), -lastName, createdAt, -createdAt
}

// ListResult is one page of employees.
type ListResult struct {
	Items    []Employee `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

// CreateResult is what a successful create returns.
type CreateResult struct {
	Employee             Employee `json:"employee"`
	ActivationLinkIssued bool     `json:"activationLinkIssued"`
}
