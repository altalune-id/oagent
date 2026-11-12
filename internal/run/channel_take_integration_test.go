//go:build integration

package run_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type chat struct {
	s            scope
	conn, person uuid.UUID
	kind         string
}

func (f *pgFixture) chat(t *testing.T, s scope, kind string) chat {
	t.Helper()
	c := chat{s: s, conn: uuid.New(), person: f.endUser(t, s), kind: kind}
	f.exec(t, `INSERT INTO `+f.schema+`.channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project, device_id,
		signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', now(), now())`,
		c.conn, "c"+c.conn.String()[:12], s.org, s.project, s.agent)
	return c
}

func (f *pgFixture) arrive(t *testing.T, c chat, ext, text string, at time.Time) uuid.UUID {
	t.Helper()
	body, err := json.Marshal(map[string]any{"id": ext, "role": "user", "content": text})
	require.NoError(t, err)
	cm := run.ChannelMessage{
		ProjectID: c.s.project, AgentID: c.s.agent, ConnectionID: c.conn, ChatKey: "cht_" + c.conn.String()[:16], Kind: c.kind,
		Message: run.Message{Body: body, SpeakerID: &c.person, ExternalMessageID: ext, ChannelTime: &at}, SenderCap: 1000,
	}
	if c.kind == "dm" {
		cm.Actor, cm.ReplyVia = c.person, "wa_phone"
	} else {
		cm.ChannelAddress = "120363012345678901@g.us"
	}
	got, err := f.store.AppendChannelMessage(in(c.s.org), cm)
	require.NoError(t, err)
	return got.ThreadID
}

func externalIDs(msgs []run.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ExternalMessageID
	}
	return out
}

func (f *pgFixture) take(t *testing.T, s scope, thread uuid.UUID) (*run.Run, []run.Message, error) {
	t.Helper()
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	taken, err := f.store.StartRun(in(s.org), token, r)
	if err == nil {
		require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
		require.NoError(t, f.store.Settle(in(s.org), token, r, run.Usage{}, nil))
	}
	require.NoError(t, f.store.Release(in(s.org), thread, token))
	return r, taken, err
}

func TestIngest_BurstIsOrdered(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	t0 := time.Now().Add(-time.Minute).Truncate(time.Second)
	var thread uuid.UUID
	for _, step := range []struct {
		ext string
		sec int
	}{{"m3", 3}, {"m1", 1}, {"m4", 4}, {"m0", 0}, {"m2a", 2}, {"m2b", 2}} {
		thread = f.arrive(t, c, step.ext, step.ext, t0.Add(time.Duration(step.sec)*time.Second))
	}
	_, taken, err := f.take(t, s, thread)
	require.NoError(t, err)
	assert.Equal(t, []string{"m0", "m1", "m2a", "m2b", "m3", "m4"}, externalIDs(taken), "WhatsApp order, arrival breaking ties")
}

func TestRun_StaleMessageSkippedNoRunNoReply(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	thread := f.arrive(t, c, "old", "hours late", time.Now().Add(-2*time.Hour))
	f.arrive(t, c, "new", "fresh", time.Now())
	_, taken, err := f.take(t, s, thread)
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, externalIDs(taken))
	var status, skip string
	var runID *uuid.UUID
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT status, skip_reason, run_id FROM `+f.schema+`.messages WHERE external_message_id = 'old'`).
		Scan(&status, &skip, &runID))
	assert.Equal(t, "consumed", status)
	assert.Equal(t, "stale", skip)
	assert.Nil(t, runID, "a stale message belongs to no run")
	msgs, err := f.store.ThreadMessages(in(s.org), thread, 0, 10)
	require.NoError(t, err)
	reasons := map[string]string{}
	for _, m := range msgs {
		reasons[m.ExternalMessageID] = m.SkipReason
	}
	assert.Equal(t, map[string]string{"old": run.SkipStale, "new": ""}, reasons, "a stale message reads back with its reason")

	other := f.chat(t, s, "dm")
	allStale := f.arrive(t, other, "late1", "a", time.Now().Add(-time.Hour))
	f.arrive(t, other, "late2", "b", time.Now().Add(-40*time.Minute))
	_, _, err = f.take(t, s, allStale)
	require.ErrorIs(t, err, run.ErrNothingPending, "an all-stale take starts no run")
	assert.Zero(t, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE thread_id = $1`, allStale))
	assert.Equal(t, 2, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND skip_reason = 'stale'`, allStale),
		"the stale marks commit even with no run")
}

func TestRun_TurnCapDefersRest(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	t0 := time.Now().Add(-time.Minute)
	var thread uuid.UUID
	for i := range 21 {
		thread = f.arrive(t, c, "m"+strconv.Itoa(100+i), "x", t0.Add(time.Duration(i)*time.Millisecond))
	}
	_, taken, err := f.take(t, s, thread)
	require.NoError(t, err)
	assert.Len(t, taken, 20, "at most 20 messages a turn")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND status = 'pending'`, thread), "the rest waits")
	_, taken, err = f.take(t, s, thread)
	require.NoError(t, err)
	assert.Equal(t, []string{"m120"}, externalIDs(taken), "the next turn takes it")

	big := f.chat(t, s, "dm")
	long := strings.Repeat("y", 15<<10)
	var bigThread uuid.UUID
	for i := range 3 {
		bigThread = f.arrive(t, big, "b"+strconv.Itoa(i), long, t0.Add(time.Duration(i)*time.Second))
	}
	_, taken, err = f.take(t, s, bigThread)
	require.NoError(t, err)
	assert.Equal(t, []string{"b0", "b1"}, externalIDs(taken), "at most 32 KiB of text a turn")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND status = 'pending'`, bigThread))
}

func TestRun_OneHugeMessageStillTaken(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	thread := f.arrive(t, c, "huge", strings.Repeat("z", 40<<10), time.Now())
	_, taken, err := f.take(t, s, thread)
	require.NoError(t, err)
	assert.Equal(t, []string{"huge"}, externalIDs(taken), "a message over the cap alone is never stuck")
}

func TestStore_QuietWaitReadsTheConnectionWindow(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	f.exec(t, `UPDATE `+f.schema+`.channel_connections SET quiet_window_ms = 3000, quiet_max_ms = 10000 WHERE id = $1`, c.conn)
	thread := f.arrive(t, c, "m1", "hi", time.Now())
	ws := f.store.(run.Windows)
	wait, err := ws.QuietWait(in(s.org), thread, time.Now())
	require.NoError(t, err)
	assert.InDelta(t, float64(3*time.Second), float64(wait), float64(time.Second), "a fresh message waits about W")
	wait, err = ws.QuietWait(in(s.org), thread, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, wait, "an old pending message waits no more")

	f.exec(t, `UPDATE `+f.schema+`.channel_connections SET quiet_window_ms = 0, quiet_max_ms = 0 WHERE id = $1`, c.conn)
	wait, err = ws.QuietWait(in(s.org), thread, time.Now())
	require.NoError(t, err)
	assert.Zero(t, wait, "a zero window never waits")

	api := f.thread(t, s)
	f.say(t, s, api)
	wait, err = ws.QuietWait(in(s.org), api, time.Now())
	require.NoError(t, err)
	assert.Zero(t, wait, "an API thread never waits")
	wait, err = ws.QuietWait(in(f.seed(t).org), thread, time.Now())
	require.NoError(t, err)
	assert.Zero(t, wait, "another tenant sees nothing to wait for")
}

func TestStore_RunRowMarksTheGroup(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	group := f.arrive(t, f.chat(t, s, "group"), "g1", "halo", time.Now())
	token := f.claimed(t, s, group)
	r := run.New(s.org, s.project, group, s.version, time.Now())
	r.Group = true
	_, err := f.store.StartRun(in(s.org), token, r)
	require.NoError(t, err)
	got, err := f.store.RunByID(in(s.org), r.ID)
	require.NoError(t, err)
	assert.True(t, got.Group, "a group run is marked on its row")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND is_group`, r.ID))
}

func TestStore_ThreadCarriesItsConnection(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	c := f.chat(t, s, "dm")
	thread := f.arrive(t, c, "d1", "halo", time.Now())
	th, err := f.store.Thread(in(s.org), thread)
	require.NoError(t, err)
	assert.Equal(t, c.conn, th.ConnectionID)

	f.exec(t, `DELETE FROM `+f.schema+`.channel_connections WHERE id = $1`, c.conn)
	th, err = f.store.Thread(in(s.org), thread)
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, th.ConnectionID, "a detached thread has no connection")
}

func TestWhatsAppApproval_DecidesInChannelOrderOnPostgres(t *testing.T) {
	p := newPgRunner(t)
	a := &fakes.RunApprovals{}
	p.svc.WithApprovals(a)
	c := p.chat(t, p.s, "dm")
	now := time.Now().Add(-time.Minute)
	thread := p.arrive(t, c, "wamid-1", "record lunch 25000", now)
	parked, err := p.turn(t, thread, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
	require.NoError(t, err)
	require.Equal(t, run.StatusAwaitingApproval, parked.Status)
	var last int64
	require.NoError(t, p.admin.QueryRowContext(t.Context(), `SELECT max(seq) FROM `+p.schema+`.messages WHERE thread_id = $1`, thread).Scan(&last))
	a.PromptSent(parked.ID, "", last)

	p.arrive(t, c, "wamid-3", "tidak", now.Add(2*time.Second))
	p.arrive(t, c, "wamid-2", "ya", now.Add(time.Second))
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, parked.ID, r.ID)
	assert.Equal(t, run.StatusCompleted, r.Status)
	answers := a.Answers()
	require.Len(t, answers, 1)
	assert.True(t, answers[0].Approved, "the earlier WhatsApp time decides, not the earlier insert")

	pending, err := p.store.PendingMessages(in(p.s.org), thread, c.person, last, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"wamid-3"}, externalIDs(pending), "the later message waits")
	var runID uuid.UUID
	require.NoError(t, p.admin.QueryRowContext(t.Context(), `SELECT run_id FROM `+p.schema+`.messages WHERE external_message_id = 'wamid-2'`).Scan(&runID))
	assert.Equal(t, parked.ID, runID, "the answer is the run's message")

	var lost *run.LeaseLostError
	require.ErrorAs(t, p.store.AnswerMessage(in(p.s.org), "not-the-lease", thread, uuid.New(), parked.ID), &lost, "only the lease holder consumes an answer")
}
