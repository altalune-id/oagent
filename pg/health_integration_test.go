//go:build integration

package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/pg"
)

func TestHealthMonitor_ProbesASingleHandleOnce(t *testing.T) {
	sqlDB := openReplica(t, pgtest.New(t).DSN)
	m := pg.NewHealthMonitor(sqlDB, sqlDB, pg.HealthConfig{}, nil, nil)
	require.Nil(t, m.Snapshot())
	require.False(t, m.Ready())

	require.NoError(t, m.Probe(t.Context()))
	snap := m.Snapshot()
	require.NotNil(t, snap)
	assert.True(t, snap.Ready)
	assert.True(t, m.Ready())
	require.Len(t, snap.Handles, 1, "a reader aliasing the writer is probed once")
	assert.Equal(t, "writer", snap.Handles[0].Name)
	assert.True(t, snap.Handles[0].Up)
	assert.Empty(t, snap.Handles[0].Err)
	assert.Positive(t, snap.Handles[0].Latency)
	assert.Positive(t, snap.Handles[0].Stats.OpenConnections)
	assert.Same(t, time.UTC, snap.At.Location())
}

func TestHealthMonitor_ReportsADeadReader(t *testing.T) {
	dsn := pgtest.New(t).DSN
	writer := openReplica(t, dsn)
	reader, err := pg.Open(t.Context(), pg.Config{DSN: dsn}, nil)
	require.NoError(t, err)
	m := pg.NewHealthMonitor(writer, reader, pg.HealthConfig{Timeout: 5 * time.Second}, nil, nil)

	require.NoError(t, m.Probe(t.Context()))
	require.Len(t, m.Snapshot().Handles, 2)
	assert.True(t, m.Ready())

	require.NoError(t, reader.Close())
	err = m.Probe(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pg: probe reader")
	assert.NotContains(t, err.Error(), "probe writer")

	snap := m.Snapshot()
	assert.False(t, snap.Ready)
	assert.False(t, m.Ready())
	require.Len(t, snap.Handles, 2)
	assert.Equal(t, "writer", snap.Handles[0].Name)
	assert.True(t, snap.Handles[0].Up)
	assert.Equal(t, "reader", snap.Handles[1].Name)
	assert.False(t, snap.Handles[1].Up)
	assert.NotEmpty(t, snap.Handles[1].Err)
}

func TestHealthMonitor_RunProbesImmediatelyAndStopsWithItsContext(t *testing.T) {
	sqlDB := openReplica(t, pgtest.New(t).DSN)
	m := pg.NewHealthMonitor(sqlDB, sqlDB, pg.HealthConfig{Interval: time.Hour}, nil, nil)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	require.Eventually(t, m.Ready, 10*time.Second, 10*time.Millisecond, "Run must probe before the first interval")
	cancel()
	require.NoError(t, <-done)
}
