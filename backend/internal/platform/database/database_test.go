package database

import (
	"context"
	"endosoflow/internal/platform/ids"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// These tests need a migrated database: run `make db-verify` once, then `make test`.
func testPool(t *testing.T) (context.Context, TxStarter, func(sql string, args ...any) int) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	count := func(sql string, args ...any) int {
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	return ctx, pool, count
}

const insertAccessLog = `INSERT INTO access_logs (account_type, action_type, attempted_document_number)
	VALUES ('employee', 'login_failed', $1)`

const countAccessLog = `SELECT count(*) FROM access_logs WHERE attempted_document_number = $1`

func TestWithTxRollsBackWhenTheFunctionFails(t *testing.T) {
	ctx, pool, count := testPool(t)
	marker := ids.NewString()
	sentinel := errors.New("boom")

	err := WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, insertAccessLog, marker); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the sentinel error", err)
	}
	if n := count(countAccessLog, marker); n != 0 {
		t.Fatalf("row was kept after rollback (%d rows)", n)
	}
}

func TestWithTxRollsBackOnPanicAndRepanics(t *testing.T) {
	ctx, pool, count := testPool(t)
	marker := ids.NewString()

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected the panic to propagate")
			}
		}()
		_ = WithTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, insertAccessLog, marker); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	if n := count(countAccessLog, marker); n != 0 {
		t.Fatalf("row was kept after a panic (%d rows)", n)
	}
}

func TestWithTxCommitsWhenTheFunctionSucceeds(t *testing.T) {
	ctx, pool, count := testPool(t)
	marker := ids.NewString()

	err := WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, insertAccessLog, marker)
		return err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := count(countAccessLog, marker); n != 1 {
		t.Fatalf("expected 1 committed row, got %d", n)
	}
}

func TestAfterCommitRunsOnlyOnceTheChangeIsVisible(t *testing.T) {
	ctx, pool, count := testPool(t)
	marker := ids.NewString()

	var visibleInHook int
	ran := 0
	err := WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, insertAccessLog, marker); err != nil {
			return err
		}
		AfterCommit(tx, func() {
			ran++
			visibleInHook = count(countAccessLog, marker) // read from another connection
		})
		if ran != 0 {
			t.Fatal("the hook must not run before the commit")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if ran != 1 || visibleInHook != 1 {
		t.Fatalf("hook ran %d times and saw %d rows; want once, after the commit", ran, visibleInHook)
	}
}

func TestAfterCommitDoesNotRunOnRollback(t *testing.T) {
	ctx, pool, _ := testPool(t)
	ran := false
	_ = WithTx(ctx, pool, func(tx pgx.Tx) error {
		AfterCommit(tx, func() { ran = true })
		return errors.New("boom")
	})
	if ran {
		t.Fatal("a rolled back transaction must not run its after-commit hooks")
	}
}

func TestAfterCommitRunsAtOnceOutsideATransaction(t *testing.T) {
	_, pool, _ := testPool(t)
	ran := false
	AfterCommit(pool, func() { ran = true }) // statements on the pool commit by themselves
	if !ran {
		t.Fatal("the hook must run immediately when there is no transaction to wait for")
	}
}
