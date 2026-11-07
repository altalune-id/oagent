package channel_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/channel/channeltest"
	"altalune.id/oagent/internal/run"
)

type replyStore struct {
	mu        sync.Mutex
	jobs      map[uuid.UUID]*run.ReplyJob
	next      map[uuid.UUID]time.Time
	failParts int
}

func (s *replyStore) ReplyJob(_ context.Context, id uuid.UUID) (*run.ReplyJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, &run.NotFoundError{ID: id.String()}
	}
	c := *j
	return &c, nil
}

func (s *replyStore) DueReplies(_ context.Context, now time.Time, _ int) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []uuid.UUID
	for id, j := range s.jobs {
		if j.State == run.ReplyDue && !s.next[id].After(now) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *replyStore) ReplyProgress(_ context.Context, id uuid.UUID, parts int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failParts > 0 {
		s.failParts--
		return errors.New("db down")
	}
	s.jobs[id].PartsSent = parts
	return nil
}

func (s *replyStore) ReplyRetry(_ context.Context, id uuid.UUID, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id].Attempts++
	s.next[id] = next
	return nil
}

func (s *replyStore) ReplyEnd(_ context.Context, id uuid.UUID, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id].State = state
	return nil
}

func (s *replyStore) state(id uuid.UUID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[id].State
}

type conns struct {
	mu       sync.Mutex
	conn     *channel.Connection
	recorded []string
}

func (c *conns) Open(_ context.Context, id uuid.UUID) (*channel.Connection, channel.Secrets, error) {
	if c.conn == nil || c.conn.ID != id {
		return nil, channel.Secrets{}, &channel.NotFoundError{}
	}
	return c.conn, channel.Secrets{DeviceKey: "owk_device"}, nil
}

func (c *conns) Record(_ context.Context, _ uuid.UUID, result string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recorded = append(c.recorded, result)
	return nil
}

type scripted struct {
	*channeltest.Fake
	mu   sync.Mutex
	errs []error
	keys []string
}

func (p *scripted) Send(ctx context.Context, c channel.Connection, s channel.Secrets, r channel.Reply) (string, error) {
	p.mu.Lock()
	p.keys = append(p.keys, r.IdempotencyKey)
	var err error
	if len(p.errs) > 0 {
		err, p.errs = p.errs[0], p.errs[1:]
	}
	p.mu.Unlock()
	if err != nil {
		return "", err
	}
	return p.Fake.Send(ctx, c, s, r)
}

type replyWorld struct {
	store    *replyStore
	conns    *conns
	provider *scripted
	logs     *bytes.Buffer
	replier  *channel.Replier
	run      uuid.UUID
}

const phone = "+6281234567890"

func newReplyWorld(t *testing.T, kind, text string) *replyWorld {
	t.Helper()
	conn := &channel.Connection{ID: uuid.New(), DeviceID: "dev_V1StGXR8Z5jdHi6B"}
	w := &replyWorld{
		store:    &replyStore{jobs: map[uuid.UUID]*run.ReplyJob{}, next: map[uuid.UUID]time.Time{}},
		conns:    &conns{conn: conn},
		provider: &scripted{Fake: &channeltest.Fake{}},
		logs:     &bytes.Buffer{},
		run:      uuid.New(),
	}
	job := &run.ReplyJob{RunID: w.run, RunPublicID: "runpub", ConnectionID: conn.ID, Kind: kind, To: phone, Text: text, State: run.ReplyDue, EndedAt: time.Now()}
	if kind == "group" {
		job.To, job.QuoteID = "120363012345678901@g.us", "wamid-9"
	}
	w.store.jobs[w.run] = job
	log := slog.New(slog.NewTextHandler(w.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w.replier = channel.NewReplier(w.conns, w.store, w.provider, log)
	return w
}

func twoParts() string { return strings.Repeat("a", 3000) + "\n\n" + strings.Repeat("b", 3000) }

func TestReply_SendsAndEnds(t *testing.T) {
	w := newReplyWorld(t, "dm", "**halo**")
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	sent := w.provider.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, channel.Reply{To: phone, Text: "*halo*", IdempotencyKey: "run:runpub:0"}, sent[0])
	assert.Equal(t, run.ReplySent, w.store.state(w.run))
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Len(t, w.provider.Sent(), 1, "a sent reply is never sent again")
}

func TestReply_KeyPerPartRetrySendsRest(t *testing.T) {
	w := newReplyWorld(t, "dm", twoParts())
	w.provider.errs = []error{nil, &channel.ProviderUnavailableError{Cause: errors.New("status 503")}}
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Equal(t, run.ReplyDue, w.store.state(w.run))
	assert.Equal(t, 1, w.store.jobs[w.run].PartsSent)
	assert.Equal(t, 1, w.store.jobs[w.run].Attempts)

	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Equal(t, []string{"run:runpub:0", "run:runpub:1", "run:runpub:1"}, w.provider.keys, "the retry sends only the rest, under its own key")
	assert.Len(t, w.provider.Sent(), 2)
	assert.Equal(t, run.ReplySent, w.store.state(w.run))
}

func TestReply_CrashRetryReusesKeys(t *testing.T) {
	w := newReplyWorld(t, "dm", "halo")
	w.store.failParts = 1
	require.Error(t, w.replier.Send(t.Context(), w.run), "the progress write failed after the send")
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Equal(t, []string{"run:runpub:0", "run:runpub:0"}, w.provider.keys, "the resend reuses the key, so the provider replays it")
	assert.Len(t, w.provider.Sent(), 1)
}

func TestReply_Key401RecordsKeyRefused(t *testing.T) {
	w := newReplyWorld(t, "dm", "halo")
	w.provider.errs = []error{&channel.KeyRefusedError{}}
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Equal(t, []string{channel.ResultKeyRefused}, w.conns.recorded)
	assert.Equal(t, run.ReplyDue, w.store.state(w.run), "it is retried")
}

func TestReply_GroupQuotesDMDoesNot(t *testing.T) {
	g := newReplyWorld(t, "group", twoParts())
	require.NoError(t, g.replier.Send(t.Context(), g.run))
	sent := g.provider.Sent()
	require.Len(t, sent, 2)
	assert.Equal(t, "wamid-9", sent[0].ReplyTo, "the first part quotes the message it answers")
	assert.Empty(t, sent[1].ReplyTo)
	assert.Equal(t, "120363012345678901@g.us", sent[0].To)

	d := newReplyWorld(t, "dm", "halo")
	d.store.jobs[d.run].QuoteID = "wamid-9"
	require.NoError(t, d.replier.Send(t.Context(), d.run))
	assert.Empty(t, d.provider.Sent()[0].ReplyTo, "a DM never quotes")
}

func TestReply_DetachedThreadIsSkipped(t *testing.T) {
	w := newReplyWorld(t, "dm", "halo")
	w.store.jobs[w.run].ConnectionID = uuid.Nil
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Empty(t, w.provider.Sent())
	assert.Equal(t, run.ReplyFailed, w.store.state(w.run))
	assert.Contains(t, w.logs.String(), "detached")
}

func TestReply_DisabledConnectionIsNotSent(t *testing.T) {
	w := newReplyWorld(t, "dm", "halo")
	w.conns.conn = nil
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Empty(t, w.provider.Sent())
	assert.Equal(t, run.ReplyFailed, w.store.state(w.run))
}

func TestReply_NoPhoneInLogs(t *testing.T) {
	w := newReplyWorld(t, "dm", "halo")
	w.provider.errs = []error{&channel.ProviderUnavailableError{Cause: errors.New("status 500")}}
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.NotEmpty(t, w.logs.String())
	assert.NotContains(t, w.logs.String(), "6281234567890")
	assert.NotContains(t, w.logs.String(), "halo", "nor the reply text")
}

func TestReplyJob_GivesUpAfter10Min(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newReplyWorld(t, "dm", "halo")
		start := time.Now()
		w.store.jobs[w.run].EndedAt = start
		failures := make([]error, 1000)
		for i := range failures {
			failures[i] = &channel.ProviderUnavailableError{Cause: errors.New("status 503")}
		}
		w.provider.errs = failures
		job := w.replier.SchedulerJobs()[0]
		assert.Equal(t, "channel-reply", job.Name)
		for w.store.state(w.run) == run.ReplyDue {
			require.NoError(t, job.Run(t.Context()))
			time.Sleep(15 * time.Second)
			require.Less(t, time.Since(start), 15*time.Minute, "it gives up")
		}
		assert.Equal(t, run.ReplyFailed, w.store.state(w.run))
		assert.GreaterOrEqual(t, time.Since(start), 10*time.Minute, "never before 10 min")
		assert.Contains(t, w.logs.String(), "giving up")
	})
}
