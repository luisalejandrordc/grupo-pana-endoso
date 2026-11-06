package tasks

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Every query aliases task_instances as t and task_definitions as d.
const taskColumns = `t.id, t.task_number, d.name_es, d.milestone_number, t.status::text, t.started_at, t.completed_at,
	t.due_date, t.completion_phase::text, d.recommended_duration_hours, d.reminder_interval_hours, t.notes, t.row_version`

const taskFrom = ` FROM task_instances t JOIN task_definitions d ON d.task_number = t.task_number`

// taskRow is a task plus the reminder interval, which the API does not show.
type taskRow struct {
	Task
	intervalHours *int
}

func scanTask(row pgx.Row) (*taskRow, error) {
	var t taskRow
	err := row.Scan(&t.ID, &t.Number, &t.Name, &t.MilestoneNumber, &t.Status, &t.StartedAt, &t.CompletedAt,
		&t.DueDate, &t.CompletionPhase, &t.DurationHours, &t.intervalHours, &t.Notes, &t.Version)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// findTask returns (nil, nil) when the sale has no such task. Eligible is not filled in here.
func findTask(ctx context.Context, q Querier, saleID uuid.UUID, number int, forUpdate bool) (*taskRow, error) {
	sql := `SELECT ` + taskColumns + taskFrom + ` WHERE t.sale_id = $1 AND t.task_number = $2`
	if forUpdate {
		sql += ` FOR UPDATE OF t`
	}
	t, err := scanTask(q.QueryRow(ctx, sql, saleID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// listTasks returns the 13 tasks of a sale in order, with Eligible filled in.
func listTasks(ctx context.Context, q Querier, saleID uuid.UUID) ([]taskRow, error) {
	rows, err := q.Query(ctx, `SELECT `+taskColumns+taskFrom+` WHERE t.sale_id = $1 ORDER BY t.task_number`, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []taskRow
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Eligible = out[i].Status == StatusPending && (i == 0 || out[i-1].Status == StatusCompleted)
	}
	return out, nil
}

// previousCompleted reports whether the task before this one is completed (always true for Task 1).
func previousCompleted(ctx context.Context, q Querier, saleID uuid.UUID, number int) (bool, error) {
	if number == 1 {
		return true, nil
	}
	var done bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM task_instances
		WHERE sale_id = $1 AND task_number = $2 AND status = 'completed')`, saleID, number-1).Scan(&done)
	return done, err
}

// highestCompleted returns the highest completed task number, or 0 when none is completed.
func highestCompleted(ctx context.Context, q Querier, saleID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT COALESCE(max(task_number), 0) FROM task_instances
		WHERE sale_id = $1 AND status = 'completed'`, saleID).Scan(&n)
	return n, err
}

func anyInProgress(ctx context.Context, q Querier, saleID uuid.UUID) (bool, error) {
	var busy bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM task_instances WHERE sale_id = $1 AND status = 'in_progress')`,
		saleID).Scan(&busy)
	return busy, err
}
