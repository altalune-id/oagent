package db

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunInTx_NilPool(t *testing.T) {
	err := RunInTx(context.Background(), Pool{}, func(context.Context) error { return nil })
	assert.Error(t, err)
}

func TestRunInTx_Nested(t *testing.T) {
	ctx := ContextWithTx(context.Background(), nil)
	err := RunInTx(ctx, Pool{}, func(context.Context) error { return nil })
	assert.ErrorIs(t, err, ErrNestedUnitOfWork)
}

func TestRunInTx_Commit(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	err = RunInTx(context.Background(), Pool{W: sqlDB}, func(ctx context.Context) error {
		_, ok := CurrentTx(ctx)
		assert.True(t, ok)
		return nil
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_Rollback(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()

	err = RunInTx(context.Background(), Pool{W: sqlDB}, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_BeginError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	mock.ExpectBegin().WillReturnError(errors.New("no tx"))

	err = RunInTx(context.Background(), Pool{W: sqlDB}, func(context.Context) error { return nil })
	assert.Error(t, err)
}

func TestContextWithTx(t *testing.T) {
	_, ok := CurrentTx(context.Background())
	assert.False(t, ok)

	ctx := ContextWithTx(context.Background(), nil)
	_, ok = CurrentTx(ctx)
	assert.True(t, ok)
}
