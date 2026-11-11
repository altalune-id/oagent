package pg_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/pg"
	"altalune.id/oagent/worker"
)

func testHealthLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func healthCfg() pg.HealthConfig {
	return pg.HealthConfig{Interval: 30 * time.Second, Timeout: 2 * time.Second}
}

func TestHealthMonitor_ProbeSuccess(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing()

	m := pg.NewHealthMonitor(sqlDB, sqlDB, healthCfg(), testHealthLogger(), nil)
	require.NoError(t, m.Probe(t.Context()))

	require.True(t, m.Ready())
	snap := m.Snapshot()
	require.NotNil(t, snap)
	require.True(t, snap.Ready)
	require.Len(t, snap.Handles, 1, "aliased handles are probed once")
	require.Equal(t, "writer", snap.Handles[0].Name)
	require.True(t, snap.Handles[0].Up)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHealthMonitor_ProbeFailureClearsReadiness(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing().WillReturnError(context.DeadlineExceeded)

	m := pg.NewHealthMonitor(sqlDB, sqlDB, healthCfg(), testHealthLogger(), nil)
	require.Error(t, m.Probe(t.Context()))

	require.False(t, m.Ready())
	snap := m.Snapshot()
	require.NotNil(t, snap)
	require.False(t, snap.Ready)
	require.False(t, snap.Handles[0].Up)
	require.NotEmpty(t, snap.Handles[0].Err)
}

func TestHealthMonitor_ProbesEachDistinctHandle(t *testing.T) {
	writer, wm, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	reader, rm, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	wm.ExpectPing()
	rm.ExpectPing()

	m := pg.NewHealthMonitor(writer, reader, healthCfg(), testHealthLogger(), nil)
	require.NoError(t, m.Probe(t.Context()))

	snap := m.Snapshot()
	require.Len(t, snap.Handles, 2)
	require.Equal(t, "writer", snap.Handles[0].Name)
	require.Equal(t, "reader", snap.Handles[1].Name)
}

func TestHealthMonitor_IsNotABackgroundWorker(t *testing.T) {
	var m any = pg.NewHealthMonitor(nil, nil, healthCfg(), testHealthLogger(), nil)
	_, isWorker := m.(worker.Worker)
	require.False(t, isWorker, "no loop may ping the database without a probe")
}

func TestHealthMonitor_ReadyProbesOnDemandAndCaches(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing()

	m := pg.NewHealthMonitor(sqlDB, sqlDB, healthCfg(), testHealthLogger(), nil)
	require.Nil(t, m.Snapshot(), "construction must not ping")
	require.True(t, m.Ready(), "the first Ready probes")
	require.True(t, m.Ready(), "a fresh snapshot is served from the cache")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHealthMonitor_ReadyReprobesAStaleSnapshot(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing()
	mock.ExpectPing().WillReturnError(context.DeadlineExceeded)

	m := pg.NewHealthMonitor(sqlDB, sqlDB,
		pg.HealthConfig{Interval: time.Millisecond, Timeout: time.Second}, testHealthLogger(), nil)
	require.True(t, m.Ready())
	time.Sleep(5 * time.Millisecond)
	require.False(t, m.Ready(), "a snapshot older than the interval is probed again")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHealthMonitor_ReadyClampsNonPositiveConfigToDefaults(t *testing.T) {
	tests := []struct {
		name string
		cfg  pg.HealthConfig
	}{
		{"zero config", pg.HealthConfig{}},
		{"negative interval", pg.HealthConfig{Interval: -time.Minute, Timeout: 5 * time.Second}},
		{"negative timeout", pg.HealthConfig{Interval: time.Minute, Timeout: -5 * time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			mock.ExpectPing()

			m := pg.NewHealthMonitor(sqlDB, sqlDB, tt.cfg, testHealthLogger(), nil)
			require.True(t, m.Ready())
			require.True(t, m.Ready(), "a clamped interval still caches")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestHealthMonitor_ProbeSucceedsWithZeroConfiguredTimeout(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing()

	m := pg.NewHealthMonitor(sqlDB, sqlDB, pg.HealthConfig{}, testHealthLogger(), nil)
	require.NoError(t, m.Probe(t.Context()))
	require.True(t, m.Ready())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHealthMonitor_ConcurrentReadyPingsOnce(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectPing().WillDelayFor(50 * time.Millisecond)

	m := pg.NewHealthMonitor(sqlDB, sqlDB, healthCfg(), testHealthLogger(), nil)
	var wg sync.WaitGroup
	ready := make([]bool, 8)
	for i := range ready {
		wg.Go(func() { ready[i] = m.Ready() })
	}
	wg.Wait()
	for _, r := range ready {
		assert.True(t, r)
	}
	require.NoError(t, mock.ExpectationsWereMet(), "probes that wait on one in flight reuse its result")
}
