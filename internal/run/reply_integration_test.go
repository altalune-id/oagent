//go:build integration

package run_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type replyStore interface {
	ReplyJob(ctx context.Context, runID uuid.UUID) (*run.ReplyJob, error)
	DueReplies(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	ReplyProgress(ctx context.Context, runID uuid.UUID, partsSent int) error
	ReplyRetry(ctx context.Context, runID uuid.UUID, next time.Time) error
	ReplyEnd(ctx context.Context, runID uuid.UUID, state string) error
}

func (p *pgRunner) replies() replyStore { return p.store.(replyStore) }

type failingSnapshots struct{ run.SnapshotStore }

func (failingSnapshots) Load(context.Context, uuid.UUID) (*run.Snapshot, error) { return nil, nil }
func (failingSnapshots) Save(context.Context, *run.Snapshot) error {
	return errors.New("snapshot store down")
}

func (p *pgRunner) identity(t *testing.T, person uuid.UUID, kind, value string) {
	t.Helper()
	p.exec(t, `INSERT INTO `+p.schema+`.end_user_identities (id, org_id, project_id, end_user_id, kind, value, created_at) VALUES ($1, $2, $3, $4, $5, $6, now())`,
		uuid.New(), p.s.org, p.s.project, person, kind, value)
}

func (p *pgRunner) turn(t *testing.T, thread uuid.UUID, replies ...fakes.RuntimeReply) (*run.Run, error) {
	t.Helper()
	p.rt.Replies = append(p.rt.Replies, replies...)
	token := p.claimed(t, p.s, thread)
	r, err := p.svc.Execute(in(p.s.org), thread, token)
	if err == nil {
		require.NoError(t, p.store.Release(in(p.s.org), thread, token))
	}
	return r, err
}

func (p *pgRunner) replyState(t *testing.T, runID uuid.UUID) (state *string, next *time.Time) {
	t.Helper()
	require.NoError(t, p.admin.QueryRowContext(t.Context(), `SELECT reply_state, reply_next_at FROM `+p.schema+`.runs WHERE id = $1`, runID).Scan(&state, &next))
	return state, next
}

func TestSettle_ReplyStateCommitsWithTheRun(t *testing.T) {
	p := newPgRunner(t)
	p.svc.WithSnapshots(failingSnapshots{})
	thread := p.arrive(t, p.chat(t, p.s, "dm"), "wamid-1", "halo", time.Now())
	_, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.True(t, run.IsSettleError(err), "%v", err)
	var state *string
	require.NoError(t, p.admin.QueryRowContext(t.Context(), `SELECT reply_state FROM `+p.schema+`.runs WHERE thread_id = $1`, thread).Scan(&state))
	assert.Nil(t, state, "a settle that rolls back sets no reply")
}

func TestReply_VoiceOnlyDMReloadsWithTheSetReply(t *testing.T) {
	p := newPgRunner(t)
	c := p.chat(t, p.s, "dm")
	thread := p.arrive(t, c, "wamid-1", "[voice note]", time.Now())
	p.exec(t, `UPDATE `+p.schema+`.messages SET body = body || '{"channel": {"type": "audio", "voice": true}}' WHERE thread_id = $1`, thread)
	r, err := p.turn(t, thread)
	require.NoError(t, err)
	assert.Empty(t, p.rt.Requests, "no model call")
	state, next := p.replyState(t, r.ID)
	require.NotNil(t, state)
	assert.Equal(t, run.ReplyDue, *state)
	require.NotNil(t, next, "a due reply has a next try")

	msgs, err := p.store.ThreadMessages(in(p.s.org), thread, 0, 50)
	require.NoError(t, err)
	text, ok := run.LastText(msgs)
	require.True(t, ok)
	assert.Equal(t, "v", text, "the stored reply is the connection's voice text")
}

func TestStore_ReplyJobDM(t *testing.T) {
	p := newPgRunner(t)
	c := p.chat(t, p.s, "dm")
	p.identity(t, c.person, "wa_phone", "+6281234567890")
	thread := p.arrive(t, c, "wamid-1", "halo", time.Now())
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NoError(t, err)

	job, err := p.replies().ReplyJob(in(p.s.org), r.ID)
	require.NoError(t, err)
	assert.Equal(t, r.PublicID, job.RunPublicID)
	assert.Equal(t, c.conn, job.ConnectionID)
	assert.Equal(t, "dm", job.Kind)
	assert.Equal(t, "+6281234567890", job.To)
	assert.Empty(t, job.QuoteID, "a DM reply quotes nothing")
	assert.Equal(t, run.ReplyDue, job.State)
	assert.NotEmpty(t, job.Text)
	assert.Zero(t, job.PartsSent)

	require.NoError(t, p.replies().ReplyProgress(in(p.s.org), r.ID, 2))
	next := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)
	require.NoError(t, p.replies().ReplyRetry(in(p.s.org), r.ID, next))
	job, err = p.replies().ReplyJob(in(p.s.org), r.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, job.PartsSent)
	assert.Equal(t, 1, job.Attempts)
	_, at := p.replyState(t, r.ID)
	assert.True(t, next.Equal(*at))

	due, err := p.replies().DueReplies(in(p.s.org), time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, due, "not due before its next try")
	due, err = p.replies().DueReplies(in(p.s.org), next.Add(time.Second), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{r.ID}, due)

	require.NoError(t, p.replies().ReplyEnd(in(p.s.org), r.ID, run.ReplySent))
	state, at := p.replyState(t, r.ID)
	assert.Equal(t, run.ReplySent, *state)
	assert.Nil(t, at)
	require.NoError(t, p.replies().ReplyEnd(in(p.s.org), r.ID, run.ReplyFailed))
	require.NoError(t, p.replies().ReplyRetry(in(p.s.org), r.ID, next))
	state, at = p.replyState(t, r.ID)
	assert.Equal(t, run.ReplySent, *state, "only a due reply moves: a late job never undoes a sent one")
	assert.Nil(t, at)
	due, err = p.replies().DueReplies(in(p.s.org), next.Add(time.Second), 10)
	require.NoError(t, err)
	assert.Empty(t, due)
}

func TestStore_ReplyJobGroupQuotesTheLastMessage(t *testing.T) {
	p := newPgRunner(t)
	c := p.chat(t, p.s, "group")
	now := time.Now()
	p.arrive(t, c, "wamid-1", "halo", now.Add(-2*time.Second))
	thread := p.arrive(t, c, "wamid-2", "@bot stok?", now)
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NoError(t, err)
	job, err := p.replies().ReplyJob(in(p.s.org), r.ID)
	require.NoError(t, err)
	assert.Equal(t, "group", job.Kind)
	assert.Equal(t, "120363012345678901@g.us", job.To)
	assert.Equal(t, "wamid-2", job.QuoteID)
}

func TestStore_ReplyJobDetachedThread(t *testing.T) {
	p := newPgRunner(t)
	c := p.chat(t, p.s, "dm")
	p.identity(t, c.person, "wa_phone", "+6281234567890")
	thread := p.arrive(t, c, "wamid-1", "halo", time.Now())
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NoError(t, err)
	p.exec(t, `DELETE FROM `+p.schema+`.channel_connections WHERE id = $1`, c.conn)
	job, err := p.replies().ReplyJob(in(p.s.org), r.ID)
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, job.ConnectionID, "a detached thread has no connection to send through")
}
