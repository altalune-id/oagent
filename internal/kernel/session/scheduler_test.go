package session_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scheduler"
)

type failingSweep struct {
	session.Store
	err error
}

func (s failingSweep) DeleteExpired(context.Context) (int, error) { return 0, s.err }

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestScheduler_JobShape(t *testing.T) {
	jobs := session.NewScheduler(session.NewMemoryStore(), discardLog()).SchedulerJobs()
	require.Len(t, jobs, 1)
	j := jobs[0]
	assert.Equal(t, "session-sweep", j.Name)
	assert.Equal(t, scheduler.ScopeSystem, j.Scope, "sessions carry no org, so the sweep is not tenant-scoped")
	assert.True(t, j.Singleton, "one replica owns the sweep")
	assert.Equal(t, 2*time.Minute, j.Timeout)
	require.NotNil(t, j.Schedule)
	require.NotNil(t, j.Run)
}

func TestScheduler_RunSweepsExpiredSessions(t *testing.T) {
	store := session.NewMemoryStore()
	ctx := t.Context()
	require.NoError(t, store.Save(ctx, "old", session.Principal{UserID: uuid.New()}, time.Now().Add(-time.Minute)))
	require.NoError(t, store.Save(ctx, "live", session.Principal{UserID: uuid.New()}, time.Now().Add(time.Hour)))

	require.NoError(t, session.NewScheduler(store, discardLog()).SchedulerJobs()[0].Run(ctx))
	n, err := store.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "the job already removed the expired row")
	_, ok, err := store.Load(ctx, "live")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestScheduler_RunPropagatesStoreFailure(t *testing.T) {
	want := errors.New("boom")
	j := session.NewScheduler(failingSweep{err: want}, discardLog()).SchedulerJobs()[0]
	assert.ErrorIs(t, j.Run(t.Context()), want)
}
