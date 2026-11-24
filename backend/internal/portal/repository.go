package portal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaleCode builds the human-friendly sale number: the year the sale was created (Lima time) and its sequence number.
func SaleCode(createdAt time.Time, number int64) string {
	lima := time.FixedZone("America/Lima", -5*60*60)
	return fmt.Sprintf("EF-%d-%05d", createdAt.In(lima).Year(), number)
}

// saleRow is a sale as the portal reads it. There is no column for notes, the cancellation reason, the sale's
// temperature or the staff's own data: the portal cannot show what it never reads.
type saleRow struct {
	ID             uuid.UUID
	Number         int64
	CreatedAt      time.Time
	CompletedAt    *time.Time
	State          string
	ModelID        uuid.UUID
	ColorID        uuid.UUID
	Model          string
	Version        string
	Color          string
	ReferencePrice string
	Advisor        Advisor
}

const saleColumns = `s.id, s.sale_number, s.created_at, s.completed_at, s.overall_state::text,
	s.vehicle_model_id, s.vehicle_color_id,
	s.vehicle_model_name_snapshot, s.vehicle_version_name_snapshot, s.vehicle_color_name_snapshot,
	s.vehicle_reference_price_snapshot::text,
	a.first_name || ' ' || a.last_name, a.business_email, a.business_phone, a.gender::text`

func scanSale(row pgx.Row) (*saleRow, error) {
	var r saleRow
	err := row.Scan(&r.ID, &r.Number, &r.CreatedAt, &r.CompletedAt, &r.State, &r.ModelID, &r.ColorID,
		&r.Model, &r.Version, &r.Color, &r.ReferencePrice,
		&r.Advisor.Name, &r.Advisor.Email, &r.Advisor.Phone, &r.Advisor.Gender)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// customerSales are the sales of one customer, newest first.
func customerSales(ctx context.Context, q querier, customerID uuid.UUID) ([]saleRow, error) {
	rows, err := q.Query(ctx, `SELECT `+saleColumns+`
		FROM sale_registers s JOIN employees a ON a.id = s.assigned_advisor_id
		WHERE s.customer_id = $1 ORDER BY s.created_at DESC, s.id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []saleRow
	for rows.Next() {
		r, err := scanSale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// customerSale finds one sale of one customer. It returns (nil, nil) when there is none, whether the sale does
// not exist or belongs to someone else.
func customerSale(ctx context.Context, q querier, customerID, saleID uuid.UUID) (*saleRow, error) {
	r, err := scanSale(q.QueryRow(ctx, `SELECT `+saleColumns+`
		FROM sale_registers s JOIN employees a ON a.id = s.assigned_advisor_id
		WHERE s.id = $1 AND s.customer_id = $2`, saleID, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// milestonesOf reads the six milestones of each sale, with their names, from the same view the rest of the
// application uses, so the portal, the emails and the reports always agree.
func milestonesOf(ctx context.Context, q querier, saleIDs []uuid.UUID) (map[uuid.UUID][]Milestone, error) {
	rows, err := q.Query(ctx, `SELECT m.sale_id, m.milestone_number, d.name_es, d.description_es, m.state::text
		FROM v_sale_milestones m JOIN milestone_definitions d ON d.milestone_number = m.milestone_number
		WHERE m.sale_id = ANY($1) ORDER BY m.sale_id, m.milestone_number`, saleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]Milestone{}
	for rows.Next() {
		var id uuid.UUID
		var m Milestone
		if err := rows.Scan(&id, &m.Number, &m.Name, &m.Description, &m.State); err != nil {
			return nil, err
		}
		out[id] = append(out[id], m)
	}
	return out, rows.Err()
}
