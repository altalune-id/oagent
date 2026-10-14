package db

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/pg"
)

func TestLockInTx(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pg.LockKey("install")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = RunInTx(context.Background(), Pool{W: sqlDB, R: sqlDB}, func(ctx context.Context) error {
		return LockInTx(ctx, "install")
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLockInTx_NoTx(t *testing.T) {
	assert.Error(t, LockInTx(context.Background(), "install"))
}
