// Package database opens the PostgreSQL connection pool and provides the transaction helper.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a small connection pool (the hosting tiers allow few connections) and checks it works.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// TxStarter is the part of the pool that WithTx needs.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// commitHooks wraps the transaction handed to WithTx callbacks so code deep inside them can ask
// for something to happen once the transaction has really committed. Embedding pgx.Tx keeps
// every method of the transaction, so the wrapper can be used anywhere a pgx.Tx is expected.
type commitHooks struct {
	pgx.Tx
	hooks []func()
}

func (t *commitHooks) afterCommit(fn func()) { t.hooks = append(t.hooks, fn) }

// AfterCommit executes a function only after a transaction successfully commits. If the
// transaction rolls back, the function is never executed. When no transaction is involved
// (for example, using a pool or connection directly), the function executes immediately.
func AfterCommit(q any, fn func()) {
	if t, ok := q.(interface{ afterCommit(func()) }); ok {
		t.afterCommit(fn)
		return
	}
	fn()
}

// WithTx runs fn inside one transaction. It commits when fn returns nil and rolls back when fn
// returns an error or panics. Errors raised by commit-time rules (the deferred triggers) come back
// from Commit, so callers must map the returned error with apperr.FromError.
// Functions registered with AfterCommit run after a successful commit, in the order they were registered.
func WithTx(ctx context.Context, db TxStarter, fn func(tx pgx.Tx) error) error {
	raw, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	tx := &commitHooks{Tx: raw}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = raw.Rollback(context.WithoutCancel(ctx))
			panic(recovered)
		}
	}()

	if err := fn(tx); err != nil {
		rollbackErr := raw.Rollback(context.WithoutCancel(ctx))
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rollbackErr))
		}
		return err
	}

	if err := raw.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	for _, hook := range tx.hooks {
		hook()
	}
	return nil
}
