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

	page, err := f.store.ListThreads(ctx, s.project, run.ThreadFilter{}, run.Cursor{}, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []uuid.UUID{newest, middle}, []uuid.UUID{page[0].ID, page[1].ID})
	assert.Equal(t, run.StatusRunning, page[0].OpenRun, "the open run's status")
	assert.Empty(t, page[1].OpenRun, "no open run")
	assert.True(t, page[0].LastActivityAt.Equal(base.Add(2*time.Minute)))

	rest, err := f.store.ListThreads(ctx, s.project, run.ThreadFilter{}, run.Cursor{At: page[1].LastActivityAt, PublicID: page[1].PublicID}, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Equal(t, oldest, rest[0].ID)

	other := f.seed(t)
	none, err := f.store.ListThreads(ctx, s.project, run.ThreadFilter{AgentID: other.agent}, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, none, "the agent filter")
	apis, err := f.store.ListThreads(ctx, s.project, run.ThreadFilter{Kind: "api"}, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Len(t, apis, 3, "the kind filter keeps api threads")
	dms, err := f.store.ListThreads(ctx, s.project, run.ThreadFilter{Kind: "dm"}, run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, dms, "and drops every other kind")
	foreign, err := f.store.ListThreads(in(other.org), s.project, run.ThreadFilter{}, run.Cursor{}, 10)
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

func (f *pgFixture) channelThread(t *testing.T, s scope, key string) uuid.UUID {
	t.Helper()
	conn, id := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project, device_id,
		signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', now(), now())`,
		conn, "c"+conn.String()[:12], s.org, s.project, s.agent)
	f.exec(t, `INSERT INTO `+f.schema+`.threads (id, public_id, org_id, project_id, agent_id, kind, external_key, connection_id, reply_via, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'dm', $6, $7, 'wa_phone', now(), now())`, id, "t"+id.String()[:12], s.org, s.project, s.agent, key, conn)
	return id
}

func TestStore_APIThreadLookupsSeeOnlyAPIThreads(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	channel := f.channelThread(t, s, "cht_Gr5Tn8Vc1Hs6Jq0b")
	_, err := f.store.ThreadByExternalKey(ctx, s.agent, "cht_Gr5Tn8Vc1Hs6Jq0b")
	assert.True(t, run.IsThreadNotFoundError(err), "a WhatsApp thread is never an API client's thread: %v", err)

	actor := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		actor, "e"+actor.String()[:12], s.org, s.project)
	th, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "cht_Gr5Tn8Vc1Hs6Jq0b", actor)
	require.NoError(t, err)
	assert.NotEqual(t, channel, th.ID, "the API client gets its own thread with the same key")
	assert.Equal(t, "api", th.Kind)
	got, err := f.store.ThreadByExternalKey(ctx, s.agent, "cht_Gr5Tn8Vc1Hs6Jq0b")
	require.NoError(t, err)
	assert.Equal(t, th.ID, got.ID)
}

func TestStore_AppendChannelMessage(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	conn := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project, device_id,
		signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', now(), now())`,
		conn, "c"+conn.String()[:12], s.org, s.project, s.agent)
	person := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		person, "e"+person.String()[:12], s.org, s.project)
	at := time.Date(2026, 9, 28, 10, 15, 0, 0, time.UTC)
	msg := func(ext string) run.ChannelMessage {
		return run.ChannelMessage{
			ProjectID: s.project, AgentID: s.agent, ConnectionID: conn, ChatKey: "cht_Dm4Kp7Wx2Rt9Ya1c", Kind: "dm", ReplyVia: "wa_phone", Actor: person,
			Message:   run.Message{Body: []byte(`{"id":"m","role":"user","content":"hi"}`), SpeakerID: &person, ExternalMessageID: ext, ChannelTime: &at},
			SenderCap: 2,
		}
	}
	first, err := f.store.AppendChannelMessage(ctx, msg("msg_1"))
	require.NoError(t, err)
	assert.True(t, first.Pending)
	again, err := f.store.AppendChannelMessage(ctx, msg("msg_1"))
	require.NoError(t, err)
	assert.Equal(t, run.ChannelAppend{ThreadID: first.ThreadID, Duplicate: true}, again, "a known message is a no-op")
	second, err := f.store.AppendChannelMessage(ctx, msg("msg_2"))
	require.NoError(t, err)
	assert.True(t, second.Pending)
	third, err := f.store.AppendChannelMessage(ctx, msg("msg_3"))
	require.NoError(t, err)
	assert.Equal(t, first.ThreadID, third.ThreadID)
	assert.False(t, third.Pending, "past the sender cap the message is stored skipped")
	var status, skip string
	var channelTime time.Time
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT status, skip_reason, channel_time FROM `+f.schema+`.messages WHERE external_message_id = 'msg_3'`).
		Scan(&status, &skip, &channelTime))
	assert.Equal(t, "consumed", status)
	assert.Equal(t, run.SkipSenderRate, skip)
	assert.True(t, at.Equal(channelTime))

	f.exec(t, `DELETE FROM `+f.schema+`.channel_connections WHERE id = $1`, conn)
	_, err = f.store.AppendChannelMessage(ctx, msg("msg_4"))
	require.Error(t, err, "a deleted connection takes nothing")
	var detached int
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, first.ThreadID).Scan(&detached))
	assert.Equal(t, 3, detached, "the detached thread keeps its history and gets nothing new")

	_, err = f.store.AppendChannelMessage(context.Background(), msg("msg_5"))
	require.Error(t, err, "no tenant, no write")
}

func TestStore_ListAgentRunsAcrossThreads(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	var ids []uuid.UUID
	for i, thread := range []uuid.UUID{f.thread(t, s), f.thread(t, s)} {
		f.say(t, s, thread)
		r := f.startRun(t, s, thread)
		f.exec(t, `UPDATE `+f.schema+`.runs SET started_at = now() - make_interval(secs => $2) WHERE id = $1`, r.ID, 100-i)
		ids = append(ids, r.ID)
	}
	got, err := f.store.ListAgentRuns(ctx, s.agent, run.Cursor{}, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []uuid.UUID{ids[1], ids[0]}, []uuid.UUID{got[0].ID, got[1].ID}, "newest first across the agent's threads")
	page, err := f.store.ListAgentRuns(ctx, s.agent, run.Cursor{At: got[0].StartedAt, PublicID: got[0].PublicID}, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	none, err := f.store.ListAgentRuns(ctx, uuid.New(), run.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, none, "another agent's runs never show")
}

func TestStore_EndUserActivityAndThreads(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	dm := f.chat(t, s, "dm")
	dmThread := f.arrive(t, dm, "wamid-1", "halo", time.Now())
	group := f.chat(t, s, "group")
	group.person = dm.person
	groupThread := f.arrive(t, group, "wamid-2", "@bot stok?", time.Now())
	stranger := f.endUser(t, s)

	seen, err := f.store.EndUserActivity(ctx, []uuid.UUID{dm.person, stranger})
	require.NoError(t, err)
	assert.Equal(t, int64(2), seen[dm.person].Threads, "the DM it owns and the group it wrote in")
	assert.False(t, seen[dm.person].LastSeen.IsZero())
	assert.Zero(t, seen[stranger].Threads, "a person who never wrote has no threads")

	threads, err := f.store.EndUserThreads(ctx, dm.person)
	require.NoError(t, err)
	var ids []uuid.UUID
	for _, th := range threads {
		ids = append(ids, th.ID)
		assert.Equal(t, s.agent, th.AgentID)
		assert.NotEmpty(t, th.PublicID)
	}
	assert.ElementsMatch(t, []uuid.UUID{dmThread, groupThread}, ids)
	none, err := f.store.EndUserThreads(in(f.seed(t).org), dm.person)
	require.NoError(t, err)
	assert.Empty(t, none, "another org sees nothing")
}
