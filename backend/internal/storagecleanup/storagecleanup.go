// Package storagecleanup removes files from object storage after the database change that orphaned them
// has committed. A deletion is first written to storage_deletions inside the same transaction as the
// change, so a crash never loses it; Process then tries it, and whatever fails stays queued for the
// scheduled retry job.
package storagecleanup

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/platform/ids"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Reasons accepted by the storage_deletion_reason enum.
const (
	CatalogImageReplaced = "catalog_image_replaced"
	CatalogImageRemoved  = "catalog_image_removed"
	SaleDeleted          = "sale_deleted"
)

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Pending is a queued deletion.
type Pending struct {
	ID  uuid.UUID
	Key string
}

// Enqueue records that key must be deleted. Call it on the transaction of the change.
func Enqueue(ctx context.Context, q execer, key, reason string) (Pending, error) {
	id := ids.New()
	_, err := q.Exec(ctx, `INSERT INTO storage_deletions (id, storage_key, reason)
		VALUES ($1, $2, $3::text::storage_deletion_reason)`, id, key, reason)
	return Pending{ID: id, Key: key}, err
}

type database interface {
	execer
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Process tries each deletion once and records the outcome. A failure is logged and left open; it never
// fails the caller, because the business change has already committed.
func Process(ctx context.Context, db execer, store storage.Store, logger *slog.Logger, items ...Pending) {
	// The request may be over by now, but the cleanup should still run.
	ctx = context.WithoutCancel(ctx)
	for _, item := range items {
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := store.Delete(attemptCtx, item.Key)
		cancel()
		if err == nil {
			if _, markErr := db.Exec(ctx, `UPDATE storage_deletions SET completed_at = now(), attempt_count = attempt_count + 1, last_error = NULL
				WHERE id = $1`, item.ID); markErr != nil {
				logger.Error("could not mark a storage deletion as done", "id", item.ID, "error", markErr)
			}
			continue
		}
		logger.Warn("storage deletion failed; it stays queued", "key", item.Key, "error", err)
		if _, markErr := db.Exec(ctx, `UPDATE storage_deletions SET attempt_count = attempt_count + 1, last_error = $2
			WHERE id = $1`, item.ID, truncate(err.Error(), 500)); markErr != nil {
			logger.Error("could not record a storage deletion failure", "id", item.ID, "error", markErr)
		}
	}
}

// RetryOpen processes up to limit deletions that are still open, oldest first. The scheduled job calls it.
func RetryOpen(ctx context.Context, db database, store storage.Store, logger *slog.Logger, limit int) (int, error) {
	rows, err := db.Query(ctx, `SELECT id, storage_key FROM storage_deletions
		WHERE completed_at IS NULL ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var items []Pending
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.ID, &p.Key); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	Process(ctx, db, store, logger, items...)
	return len(items), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
