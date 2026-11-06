//go:build integration

package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
)

func (f *pgFixture) touch(t *testing.T, thread uuid.UUID, at time.Time) {
	t.Helper()
	f.exec(t, `UPDATE `+f.schema+`.threads SET updated_at = $2 WHERE id = $1`, thread, at)
}

func (f *pgFixture) startRun(t *testing.T, s scope, thread uuid.UUID) *run.Run {
	t.Helper()
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(in(s.org), token, r)
	require.NoError(t, err)
	return r
}

func TestStore_ResolveThreadAndRunWithoutATenant(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	f.say(t, s, thread)
	r := f.startRun(t, s, thread)
	th, err := f.store.Thread(in(s.org), thread)
	require.NoError(t, err)

	got, err := f.store.ResolveThread(context.Background(), th.PublicID)
	require.NoError(t, err)
	assert.Equal(t, run.Ref{ID: thread, OrgID: s.org, ProjectID: s.project, AgentID: s.agent}, got)
	gotRun, err := f.store.ResolveRun(context.Background(), r.PublicID)
	require.NoError(t, err)
	assert.Equal(t, run.Ref{ID: r.ID, OrgID: s.org, ProjectID: s.project, AgentID: s.agent}, gotRun, "a run carries its thread's agent")

	_, err = f.store.ResolveThread(context.Background(), "nope")
	assert.True(t, run.IsThreadNotFoundError(err), "%v", err)
	_, err = f.store.ResolveRun(context.Background(), "nope")
	assert.True(t, run.IsNotFoundError(err), "%v", err)
}

func TestStore_ListThreadsNewestActivityFirst(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	oldest, middle, newest := f.thread(t, s), f.thread(t, s), f.thread(t, s)
	f.say(t, s, newest)
	open := f.startRun(t, s, newest)
	f.touch(t, oldest, base)
	f.touch(t, middle, base.Add(time.Minute))
	f.touch(t, newest, base.Add(2*time.Minute))

	page, err := f.store.ListThreads(ctx, s.project, uuid.Nil, run.Cursor{}, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []uuid.UUID{newest, middle}, []uuid.UUID{page[0].ID, page[1].ID})
	assert.Equal(t, run.StatusRunning, page[0].OpenRun, "the open run's status")
	assert.Empty(t, page[1].OpenRun, "no open run")
	assert.True(t, page[0].LastActivityAt.Equal(base.Add(2*time.Minute)))

	rest, err := f.store.ListThreads(ctx, s.project, uuid.Nil, run.Cursor{At: page[1].LastActivityAt, PublicID: page[1].PublicID}, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Equal(t, oldest, rest[0].ID)

	other := f.seed(t)
	none, err := f.store.ListThreads(ctx, s.project, other.agent, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, none, "the agent filter")
	foreign, err := f.store.ListThreads(in(other.org), s.project, uuid.Nil, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, foreign, "another tenant sees nothing")

	f.exec(t, `UPDATE `+f.schema+`.runs SET status = 'completed', ended_at = now() WHERE id = $1`, open.ID)
	settled, err := f.store.ThreadSummary(ctx, newest)
	require.NoError(t, err)
	assert.Empty(t, settled.OpenRun)
	assert.Equal(t, int64(1), settled.Messages)
	assert.Equal(t, int64(1), settled.Runs)
	_, err = f.store.ThreadSummary(in(other.org), newest)
	assert.True(t, run.IsThreadNotFoundError(err), "%v", err)
}

func TestStore_ListRunsNewestFirst(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	var ids []uuid.UUID
	for range 3 {
		f.say(t, s, thread)
		r := f.startRun(t, s, thread)
		f.exec(t, `UPDATE `+f.schema+`.runs SET status = 'completed', ended_at = now(), started_at = now() - make_interval(secs => $2) WHERE id = $1`, r.ID, 100-len(ids))
		f.exec(t, `UPDATE `+f.schema+`.threads SET lease_owner = NULL, lease_until = NULL WHERE id = $1`, thread)
		ids = append(ids, r.ID)
	}
	page, err := f.store.ListRuns(ctx, thread, run.Cursor{}, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []uuid.UUID{ids[2], ids[1]}, []uuid.UUID{page[0].ID, page[1].ID})
	rest, err := f.store.ListRuns(ctx, thread, run.Cursor{At: page[1].StartedAt, PublicID: page[1].PublicID}, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Equal(t, ids[0], rest[0].ID)
	foreign, err := f.store.ListRuns(in(f.seed(t).org), thread, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, foreign)
}

func TestStore_LateRunScopedCancelLeavesTheNextRunAlone(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	a := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, a)
	require.NoError(t, err)
	live, err := f.store.RunByID(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, run.StatusRunning, live.Status, "the cancel reads A as live")

	require.NoError(t, a.Move(run.StatusCompleted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, a, run.Usage{}, nil))
	f.say(t, s, thread)
	b := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, b)
	require.NoError(t, err)

	require.NoError(t, f.store.RequestCancel(ctx, thread, a.ID), "the late cancel of A is a no-op")
	stop, err := f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, stop, "B is left alone")

	require.NoError(t, f.store.RequestCancel(ctx, thread, b.ID))
	stop, err = f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.True(t, stop, "a cancel of B reaches B")
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(ctx, uuid.New(), b.ID)))
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(in(f.seed(t).org), thread, b.ID)), "another tenant")
}
