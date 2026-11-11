package boot

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
)

func TestBuildDBHealthProbesOnceAtBoot(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing()

	h := buildDBHealth(context.Background(), kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}}, config.Config{}, testLogger())
	snap := h.Snapshot()
	require.NotNil(t, snap, "readiness has a value from boot")
	assert.True(t, snap.Ready)
	assert.NoError(t, mock.ExpectationsWereMet())
}
