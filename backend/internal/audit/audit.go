// Package audit writes the business audit log. Every module records its important events here,
// inside the same transaction as the change, so a change cannot commit without its audit entry.
package audit

import (
	"context"
	"encoding/json"
	"endosoflow/internal/platform/ids"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Entity types and actions. The database only accepts valid pairs (see audit_logs_valid_pair).
const (
	EntityEmployee     = "employee"
	EntityCustomer     = "customer"
	EntitySaleRegister = "sale_register"
	EntityTaskInstance = "task_instance"

	EntityVehicleModel   = "vehicle_model"
	EntityVehicleVersion = "vehicle_version"
	EntityVehicleColor   = "vehicle_color"
	EntityCatalogImage   = "catalog_image"

	ActionCreated     = "CREATED"
	ActionUpdated     = "UPDATED"
	ActionDeactivated = "DEACTIVATED"
	ActionReactivated = "REACTIVATED"
	ActionDeleted     = "DELETED"
	ActionRoleChanged = "ROLE_CHANGED"
	ActionUploaded    = "UPLOADED"
	ActionReplaced    = "REPLACED"
	ActionRemoved     = "REMOVED"
	ActionStarted     = "STARTED"
	ActionCompleted   = "COMPLETED"
	ActionRestarted   = "RESTARTED"
	ActionReopened    = "REOPENED"
	ActionCorrected   = "CORRECTED"
	ActionCancelled   = "CANCELLED"
	ActionReassigned  = "REASSIGNED"
)

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Event describes one audit entry.
type Event struct {
	ActorID    *uuid.UUID // nil means the system itself acted
	SaleID     *uuid.UUID // set for events that belong to a sale, so advisors can see their own history
	EntityType string
	EntityID   uuid.UUID
	Action     string
	Changes    map[string]any // a concise summary, never file contents or secrets
	Reason     string         // required by the database for CANCELLED, REOPENED, RESTARTED, CORRECTED and DELETED
}

// Record appends the event. It is meant to run on the transaction of the change it describes.
func Record(ctx context.Context, q execer, e Event, now time.Time) error {
	var changes *string
	if len(e.Changes) > 0 {
		encoded, err := json.Marshal(e.Changes)
		if err != nil {
			return fmt.Errorf("encode audit changes: %w", err)
		}
		text := string(encoded)
		changes = &text
	}
	var reason *string
	if e.Reason != "" {
		reason = &e.Reason
	}
	kind := "employee"
	if e.ActorID == nil {
		kind = "system"
	}
	_, err := q.Exec(ctx, `INSERT INTO audit_logs
        (id, actor_employee_id, actor_kind, sale_id, entity_type, entity_id, action,
         changes_summary, reason, created_at)
        VALUES ($1, $2, $3::text::actor_kind, $4, $5::text::audit_entity_type, $6,
                $7::text::audit_action, $8::text::jsonb, $9, $10)`,
		ids.New(), e.ActorID, kind, e.SaleID, e.EntityType, e.EntityID, e.Action, changes, reason, now)
	return err
}
