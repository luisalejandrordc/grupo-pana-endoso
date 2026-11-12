package auth

import (
	"context"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SystemActorID is the seeded employee that owns records created by the system itself.
var SystemActorID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// BootstrapAdminInput describes the very first System Admin.
type BootstrapAdminInput struct {
	DocumentType   string
	DocumentNumber string
	FirstName      string
	LastName       string
	Nationality    string
	Gender         string // M or F
	BusinessPhone  string
	BusinessEmail  string
	Birthdate      time.Time
	HireDate       time.Time
}

// BootstrapAdmin creates the first System Admin and returns their activation link. It refuses
// to run when an active System Admin already exists, so it cannot be used as a back door.
func BootstrapAdmin(ctx context.Context, svc *Service, in BootstrapAdminInput) (string, error) {
	docType, docNumber, valid := NormalizeIdentifier(in.DocumentType, in.DocumentNumber)
	if !valid {
		return "", errors.New("the document type or number is not valid (DNI: 8 digits, CE: 9 alphanumeric, PAS: 6 to 12 alphanumeric)")
	}
	email := strings.ToLower(strings.TrimSpace(in.BusinessEmail))
	gender := strings.ToUpper(strings.TrimSpace(in.Gender))
	for name, value := range map[string]string{
		"first name": strings.TrimSpace(in.FirstName), "last name": strings.TrimSpace(in.LastName),
		"nationality": strings.TrimSpace(in.Nationality), "business phone": strings.TrimSpace(in.BusinessPhone),
		"business email": email,
	} {
		if value == "" {
			return "", fmt.Errorf("%s is required", name)
		}
	}
	if gender != "M" && gender != "F" {
		return "", errors.New("gender must be M or F")
	}

	id := ids.New()
	err := database.WithTx(ctx, svc.db, func(tx pgx.Tx) error {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM employees
			WHERE role = 'system_admin' AND is_active AND NOT is_system`).Scan(&admins); err != nil {
			return err
		}
		if admins > 0 {
			return errors.New("an active System Admin already exists; create further staff from the application")
		}
		_, err := tx.Exec(ctx, `INSERT INTO employees
			(id, document_type, document_number, first_name, last_name, nationality, gender,
			 birthdate, hire_date, business_phone, business_email, role, created_by)
			VALUES ($1, $2::text::employee_document_type, $3, $4, $5, $6, $7::text::gender,
					$8, $9, $10, $11, 'system_admin', $12)`,
			id, docType, docNumber, strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName),
			strings.TrimSpace(in.Nationality), gender, in.Birthdate, in.HireDate,
			strings.TrimSpace(in.BusinessPhone), email, SystemActorID)
		return err
	})
	if err != nil {
		return "", err
	}
	systemActor := SystemActorID
	return svc.IssueActivation(ctx, id, &systemActor)
}
