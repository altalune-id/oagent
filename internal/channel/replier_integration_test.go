//go:build integration

package channel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/channel/channeltest"
	"altalune.id/oagent/internal/channel/openwa"
	"altalune.id/oagent/internal/run"
)

const replyPhone = "6281234567890"

type replyRig struct {
	*pgWorld
	s    tenantScope
	conn *channel.Connection
	runs channel.ReplyStore
}

func newReplyRig(t *testing.T, baseURL string) *replyRig {
	t.Helper()
	w := newPgWorld(t)
	w.svc.WithAllowPrivateHosts(true)
	r := &replyRig{pgWorld: w, s: w.seed(t), runs: run.NewStore(w.pool, w.schema, "")}
	settings := input()
	if baseURL != "" {
		settings.BaseURL = baseURL
	}
	conn, err := create(in(r.s), w.svc, r.s.project, r.s.agent, settings, "whsec_primary", "owk_device")
	require.NoError(t, err)
	r.conn = conn
	return r
}

func (r *replyRig) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := r.admin.ExecContext(t.Context(), strings.ReplaceAll(q, "{s}", r.schema+"."), args...)
	require.NoError(t, err)
}

func (r *replyRig) dueRun(t *testing.T, texts ...string) uuid.UUID {
	t.Helper()
	person, thread, version, runID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	r.exec(t, `INSERT INTO {s}end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		person, "e"+person.String()[:12], r.s.org, r.s.project)
	r.exec(t, `INSERT INTO {s}end_user_identities (id, org_id, project_id, end_user_id, kind, value, created_at) VALUES ($1, $2, $3, $4, 'wa_phone', $5, now())`,
		uuid.New(), r.s.org, r.s.project, person, replyPhone)
	r.exec(t, `INSERT INTO {s}threads (id, public_id, org_id, project_id, agent_id, kind, external_key, connection_id, reply_via, actor_end_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'dm', $6, $7, 'wa_phone', $8, now(), now())`,
		thread, "t"+thread.String()[:12], r.s.org, r.s.project, r.s.agent, "cht_"+thread.String()[:8], r.conn.ID, person)
	r.exec(t, `INSERT INTO {s}agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at) VALUES ($1, $2, $3, $4, $5, $6, '{}', now())`,
		version, "v"+version.String()[:12], r.s.org, r.s.project, r.s.agent, time.Now().UnixNano()%1_000_000)
	r.exec(t, `INSERT INTO {s}runs (id, public_id, org_id, project_id, thread_id, agent_version_id, status, started_at, ended_at, created_at, updated_at, reply_state, reply_next_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'completed', now(), now(), now(), now(), 'due', now())`,
		runID, "r"+runID.String()[:12], r.s.org, r.s.project, thread, version)
	seq := 1
	user, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": "user", "content": "halo"})
	r.exec(t, `INSERT INTO {s}messages (id, org_id, project_id, thread_id, seq, role, body, status, run_id, external_message_id, created_at) VALUES ($1, $2, $3, $4, $5, 'user', $6, 'consumed', $7, 'wamid-1', now())`,
		uuid.New(), r.s.org, r.s.project, thread, seq, user, runID)
	for _, text := range texts {
		seq++
		body, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": "assistant", "content": text})
		r.exec(t, `INSERT INTO {s}messages (id, org_id, project_id, thread_id, seq, role, body, status, run_id, created_at) VALUES ($1, $2, $3, $4, $5, 'assistant', $6, 'consumed', $7, now())`,
			uuid.New(), r.s.org, r.s.project, thread, seq, body, runID)
	}
	return runID
}

func (r *replyRig) replyState(t *testing.T, runID uuid.UUID) string {
	t.Helper()
	var state string
	require.NoError(t, r.admin.QueryRowContext(t.Context(), `SELECT reply_state FROM `+r.schema+`.runs WHERE id = $1`, runID).Scan(&state))
	return state
}

func (r *replyRig) lastResult(t *testing.T) string {
	t.Helper()
	var result *string
	require.NoError(t, r.admin.QueryRowContext(t.Context(), `SELECT last_result FROM `+r.schema+`.channel_connections WHERE id = $1`, r.conn.ID).Scan(&result))
	if result == nil {
		return ""
	}
	return *result
}

func TestReply_OnlyLastAssistantText(t *testing.T) {
	r := newReplyRig(t, "")
	runID := r.dueRun(t, "Sebentar, saya cek.", "Total Rp 45.000.")
	fake := &channeltest.Fake{}
	replier := channel.NewReplier(r.svc, r.runs, fake, nil)
	require.NoError(t, replier.Send(in(r.s), runID))
	sent := fake.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Total Rp 45.000.", sent[0].Text, "only the run's last assistant text is sent")
	assert.Equal(t, replyPhone, sent[0].To)
	assert.Equal(t, run.ReplySent, r.replyState(t, runID))

	require.NoError(t, replier.Send(in(r.s), runID), "a crash-retry of a sent reply is a no-op")
	assert.Len(t, fake.Sent(), 1)
}

type refusing struct{ *channeltest.Fake }

func (refusing) Send(context.Context, channel.Connection, channel.Secrets, channel.Reply) (string, error) {
	return "", &channel.KeyRefusedError{}
}

func (refusing) FetchMedia(context.Context, channel.Connection, channel.Secrets, channel.MediaRef, int64) (io.ReadCloser, string, error) {
	return nil, "", &channel.KeyRefusedError{}
}

func TestReply_Key401RecordsKeyRefusedOnTheConnection(t *testing.T) {
	r := newReplyRig(t, "")
	runID := r.dueRun(t, "halo")
	require.NoError(t, channel.NewReplier(r.svc, r.runs, refusing{&channeltest.Fake{}}, nil).Send(in(r.s), runID))
	assert.Equal(t, channel.ResultKeyRefused, r.lastResult(t))
	assert.Equal(t, run.ReplyDue, r.replyState(t, runID))
}

func TestMedia_401RecordsKeyRefused(t *testing.T) {
	r := newReplyRig(t, "")
	_, _, err := channel.Media{Service: r.svc, Provider: refusing{&channeltest.Fake{}}}.Fetch(in(r.s), r.conn.ID, "msg_1", "image/jpeg", 10)
	require.True(t, channel.IsKeyRefusedError(err))
	assert.Equal(t, channel.ResultKeyRefused, r.lastResult(t))
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestReply_NoPhoneInLogsOrSpans(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			http.Error(w, "down "+replyPhone, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"msg_Gm4Et1Op6Rs9Uv2g"}`))
	}))
	defer srv.Close()

	spans := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	r := newReplyRig(t, srv.URL)
	runID := r.dueRun(t, "halo")
	logs := &lockedBuffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	replier := channel.NewReplier(r.svc, r.runs, openwa.New(true), log)
	require.NoError(t, replier.Send(in(r.s), runID))
	require.NoError(t, replier.Send(in(r.s), runID))
	require.Equal(t, run.ReplySent, r.replyState(t, runID))

	assert.NotContains(t, logs.String(), replyPhone)
	ended := spans.Ended()
	require.NotEmpty(t, ended, "the provider's HTTP calls are traced")
	for _, s := range ended {
		assert.NotContains(t, s.Name(), replyPhone)
		for _, a := range s.Attributes() {
			assert.NotContains(t, a.Value.String(), replyPhone, "span %s attribute %s", s.Name(), a.Key)
		}
		for _, e := range s.Events() {
			for _, a := range e.Attributes {
				assert.NotContains(t, a.Value.String(), replyPhone)
			}
		}
	}
}
