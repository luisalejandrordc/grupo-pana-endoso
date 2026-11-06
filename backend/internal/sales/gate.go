package sales

import (
	"context"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/tasks"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Gate gives the task engine access to the sale it belongs to. main passes it to the tasks service as its
// tasks.SaleGate.
type Gate struct{}

var _ tasks.SaleGate = Gate{}

var identifierColumns = map[tasks.Identifier]string{
	tasks.IdentSeries: "vehicle_series",
	tasks.IdentVIN:    "vin",
	tasks.IdentPlate:  "license_plate",
}

// Find returns the sale for the task engine, or (nil, nil) when it does not exist.
func (Gate) Find(ctx context.Context, q tasks.Querier, id uuid.UUID, forUpdate bool) (*tasks.SaleRef, error) {
	sql := `SELECT id, overall_state::text, assigned_advisor_id, customer_type_snapshot::text, created_at,
		vehicle_series, vin, license_plate FROM sale_registers WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	var ref tasks.SaleRef
	err := q.QueryRow(ctx, sql, id).Scan(&ref.ID, &ref.State, &ref.AssignedAdvisorID, &ref.CustomerType,
		&ref.CreatedAt, &ref.Series, &ref.VIN, &ref.Plate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// SetIdentifier writes the series, VIN or plate of the sale.
func (Gate) SetIdentifier(ctx context.Context, q tasks.Querier, id uuid.UUID, field tasks.Identifier, value *string, actor uuid.UUID, now time.Time) error {
	column, ok := identifierColumns[field]
	if !ok {
		return apperr.Internal(errors.New("unknown sale identifier " + string(field)))
	}
	_, err := q.Exec(ctx, `UPDATE sale_registers SET `+column+` = $2, updated_by = $3, updated_at = $4,
		row_version = row_version + 1 WHERE id = $1`, id, value, actor, now)
	return err
}

// MarkCompleted moves an active sale to completed.
func (Gate) MarkCompleted(ctx context.Context, q tasks.Querier, id uuid.UUID, actor uuid.UUID, now time.Time) error {
	return changeState(ctx, q, `UPDATE sale_registers SET overall_state = 'completed', completed_at = $2,
		updated_by = $3, updated_at = $2, row_version = row_version + 1
		WHERE id = $1 AND overall_state = 'active'`, id, now, actor)
}

// MarkReactivated moves a completed sale back to active.
func (Gate) MarkReactivated(ctx context.Context, q tasks.Querier, id uuid.UUID, actor uuid.UUID, now time.Time) error {
	return changeState(ctx, q, `UPDATE sale_registers SET overall_state = 'active', completed_at = NULL,
		updated_by = $3, updated_at = $2, row_version = row_version + 1
		WHERE id = $1 AND overall_state = 'completed'`, id, now, actor)
}

func changeState(ctx context.Context, q tasks.Querier, sql string, id uuid.UUID, now time.Time, actor uuid.UUID) error {
	tag, err := q.Exec(ctx, sql, id, now, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperr.Conflict("SALE_INVALID_STATE", "La venta no está en el estado que requiere esta acción.")
	}
	return nil
}
