package pg

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockKey(t *testing.T) {
	assert.Equal(t, LockKey("migrate:t_"), LockKey("migrate:t_"))
	assert.NotEqual(t, LockKey("migrate:a"), LockKey("migrate:b"))
}

func TestRunWithAdvisoryLock(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	called := false
	err = RunWithAdvisoryLock(context.Background(), sqlDB, "migrate:t_", func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunWithAdvisoryLock_FnError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))

	err = RunWithAdvisoryLock(context.Background(), sqlDB, "x", func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
}

func TestRunWithAdvisoryLock_ConnError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())

	err = RunWithAdvisoryLock(context.Background(), sqlDB, "x", func(context.Context) error { return nil })
	assert.Error(t, err)
}

func TestRunWithAdvisoryLock_LockError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnError(errors.New("boom"))

	err = RunWithAdvisoryLock(context.Background(), sqlDB, "x", func(context.Context) error { return nil })
	assert.Error(t, err)
}
