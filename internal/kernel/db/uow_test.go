package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockPool(t *testing.T) (Pool, sqlmock.Sqlmock) {
	t.Helper()
	w, wmock, err := sqlmock.New()
	require.NoError(t, err)
	r, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = w.Close()
		_ = r.Close()
	})
	return Pool{W: w, R: r}, wmock
}

func TestRunInTx_NilPool(t *testing.T) {
	err := RunInTx(context.Background(), Pool{}, func(context.Context) error { return nil })
	assert.Error(t, err)
}

func TestRunInTx_Commit(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		_, ok := CurrentTx(ctx)
		assert.True(t, ok)
		return nil
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_Rollback(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	err := RunInTx(context.Background(), pool, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_BeginError(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin().WillReturnError(errors.New("no tx"))

	err := RunInTx(context.Background(), pool, func(context.Context) error { return nil })
	assert.Error(t, err)
}

func TestRunInTx_NestedJoinsAndCommitsOnce(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		outer, _ := CurrentTx(ctx)
		return RunInTx(ctx, pool, func(ctx context.Context) error {
			inner, ok := CurrentTx(ctx)
			require.True(t, ok)
			assert.Same(t, outer, inner, "a nested unit of work must join the outer transaction")
			return nil
		})
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_NestedErrorRollsBackTheOuter(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	boom := errors.New("boom")

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		return RunInTx(ctx, pool, func(context.Context) error { return boom })
	})
	require.ErrorIs(t, err, boom)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_OwnsATransactionWhenNoneIsAmbient(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	require.NoError(t, InTx(context.Background(), pool, func(tx *sql.Tx) error {
		assert.NotNil(t, tx)
		return nil
	}))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_RollsBackItsOwnTransactionOnError(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	boom := errors.New("boom")

	require.ErrorIs(t, InTx(context.Background(), pool, func(*sql.Tx) error { return boom }), boom)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_RollsBackItsOwnTransactionOnPanic(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	assert.Panics(t, func() {
		_ = InTx(context.Background(), pool, func(*sql.Tx) error { panic("boom") })
	})
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_ReportsACommitFailure(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("lost"))

	err := InTx(context.Background(), pool, func(*sql.Tx) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db: commit")
}

func TestInTx_JoinsTheAmbientTransaction(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		outer, _ := CurrentTx(ctx)
		return InTx(ctx, pool, func(tx *sql.Tx) error {
			assert.Same(t, outer, tx, "InTx must join, never begin or commit, an ambient transaction")
			return nil
		})
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInTx_NilPool(t *testing.T) {
	assert.Error(t, InTx(context.Background(), Pool{}, func(*sql.Tx) error { return nil }))
}

func TestReaderAndWriter(t *testing.T) {
	pool, mock := mockPool(t)
	ctx := context.Background()
	assert.Same(t, pool.R, Reader(ctx, pool))
	assert.Same(t, pool.W, Writer(ctx, pool))

	mock.ExpectBegin()
	mock.ExpectCommit()
	require.NoError(t, RunInTx(ctx, pool, func(ctx context.Context) error {
		tx, _ := CurrentTx(ctx)
		assert.Same(t, tx, Reader(ctx, pool), "Reader must use the ambient transaction")
		assert.Same(t, tx, Writer(ctx, pool), "Writer must use the ambient transaction")
		return nil
	}))
}

func TestNewTransactor_RunsAUnitOfWork(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uow := NewTransactor(pool)

	require.NoError(t, uow.RunInTx(context.Background(), func(ctx context.Context) error {
		_, ok := CurrentTx(ctx)
		assert.True(t, ok)
		return uow.RunInTx(ctx, func(context.Context) error { return nil })
	}))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContextWithTx(t *testing.T) {
	_, ok := CurrentTx(context.Background())
	assert.False(t, ok)

	ctx := ContextWithTx(context.Background(), nil)
	_, ok = CurrentTx(ctx)
	assert.True(t, ok)
}
