//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func uowPool(t *testing.T) (db.Pool, *sql.DB) {
	t.Helper()
	sqlDB := pgtest.New(t).OpenDB(t)
	_, err := sqlDB.ExecContext(t.Context(), `CREATE TABLE uow_probe (v TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	return db.Pool{W: sqlDB, R: sqlDB}, sqlDB
}

func insertProbe(ctx context.Context, t *testing.T, v string) {
	t.Helper()
	tx, ok := db.CurrentTx(ctx)
	require.True(t, ok, "RunInTx must expose its transaction on ctx")
	_, err := tx.ExecContext(ctx, `INSERT INTO uow_probe (v) VALUES ($1)`, v)
	require.NoError(t, err)
}

func probeExists(t *testing.T, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, v string) bool {
	t.Helper()
	var ok bool
	require.NoError(t, q.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM uow_probe WHERE v = $1)`, v).Scan(&ok))
	return ok
}

func TestRunInTx_CommitsWhenFnSucceeds(t *testing.T) {
	pool, sqlDB := uowPool(t)
	_, ok := db.CurrentTx(t.Context())
	require.False(t, ok)

	require.NoError(t, db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		insertProbe(ctx, t, "a")
		assert.False(t, probeExists(t, sqlDB, "a"), "an uncommitted write must be invisible to other connections")
		tx, _ := db.CurrentTx(ctx)
		assert.True(t, probeExists(t, tx, "a"), "the transaction must see its own write")
		return nil
	}))
	assert.True(t, probeExists(t, sqlDB, "a"))
}

func TestRunInTx_RollsBackWhenFnFails(t *testing.T) {
	pool, sqlDB := uowPool(t)
	boom := errors.New("boom")
	err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		insertProbe(ctx, t, "a")
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.False(t, probeExists(t, sqlDB, "a"))
}

func TestRunInTx_RollsBackWhenFnPanics(t *testing.T) {
	pool, sqlDB := uowPool(t)
	assert.Panics(t, func() {
		_ = db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
			insertProbe(ctx, t, "a")
			panic("boom")
		})
	})
	assert.False(t, probeExists(t, sqlDB, "a"))
	assert.Zero(t, sqlDB.Stats().InUse, "the panicking transaction must return its connection")
}

func TestRunInTx_ReportsACommitFailure(t *testing.T) {
	pool, sqlDB := uowPool(t)
	err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		_, execErr := tx.ExecContext(ctx, `INSERT INTO uow_probe (v) VALUES ('a'), ('a')`)
		require.Error(t, execErr)
		return nil
	})
	require.Error(t, err, "committing an aborted transaction must not report success")
	assert.Contains(t, err.Error(), "db: commit")
	assert.False(t, probeExists(t, sqlDB, "a"))
}

func TestRunInTx_NestedJoinsAndCommitsOnce(t *testing.T) {
	pool, sqlDB := uowPool(t)
	require.NoError(t, db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		insertProbe(ctx, t, "outer")
		require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
			insertProbe(ctx, t, "inner")
			return nil
		}))
		assert.False(t, probeExists(t, sqlDB, "inner"), "the inner unit of work must not commit on its own")
		return nil
	}))
	assert.True(t, probeExists(t, sqlDB, "outer"))
	assert.True(t, probeExists(t, sqlDB, "inner"))
}

func TestRunInTx_InnerErrorRollsBackTheOuter(t *testing.T) {
	pool, sqlDB := uowPool(t)
	boom := errors.New("boom")
	err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		insertProbe(ctx, t, "outer")
		return db.RunInTx(ctx, pool, func(ctx context.Context) error {
			insertProbe(ctx, t, "inner")
			return boom
		})
	})
	require.ErrorIs(t, err, boom)
	assert.False(t, probeExists(t, sqlDB, "outer"), "an inner error must roll back the outer unit of work")
	assert.False(t, probeExists(t, sqlDB, "inner"))
}

func TestInTx_JoinsOrOwns(t *testing.T) {
	pool, sqlDB := uowPool(t)
	insert := func(v string) func(tx *sql.Tx) error {
		return func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), `INSERT INTO uow_probe (v) VALUES ($1)`, v)
			return err
		}
	}
	require.NoError(t, db.InTx(t.Context(), pool, insert("owned")))
	assert.True(t, probeExists(t, sqlDB, "owned"), "InTx commits the transaction it owns")

	err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		require.NoError(t, db.InTx(ctx, pool, insert("joined")))
		assert.False(t, probeExists(t, sqlDB, "joined"), "InTx must not commit an ambient transaction")
		return errors.New("abort")
	})
	require.Error(t, err)
	assert.False(t, probeExists(t, sqlDB, "joined"), "a joined InTx rolls back with the outer unit of work")
}

func TestTransactor_ComposesNestedUseCases(t *testing.T) {
	pool, sqlDB := uowPool(t)
	uow := db.NewTransactor(pool)
	inner := func(ctx context.Context) error {
		return uow.RunInTx(ctx, func(ctx context.Context) error {
			insertProbe(ctx, t, "inner")
			return nil
		})
	}
	require.NoError(t, uow.RunInTx(t.Context(), func(ctx context.Context) error {
		insertProbe(ctx, t, "outer")
		return inner(ctx)
	}))
	assert.True(t, probeExists(t, sqlDB, "outer"))
	assert.True(t, probeExists(t, sqlDB, "inner"))
}

func TestContextWithTx_BindsACallerOwnedTransaction(t *testing.T) {
	pool, sqlDB := uowPool(t)
	tx, err := sqlDB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	ctx := db.ContextWithTx(t.Context(), tx)

	got, ok := db.CurrentTx(ctx)
	require.True(t, ok)
	require.Same(t, tx, got)
	insertProbe(ctx, t, "a")
	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
		insertProbe(ctx, t, "b")
		return nil
	}))
	assert.False(t, probeExists(t, sqlDB, "b"), "joining a caller-owned transaction must not commit it")

	require.NoError(t, tx.Rollback())
	assert.False(t, probeExists(t, sqlDB, "a"))
	assert.False(t, probeExists(t, sqlDB, "b"))
}
