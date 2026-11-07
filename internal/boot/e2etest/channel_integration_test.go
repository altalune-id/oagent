//go:build integration

package e2etest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
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

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/runtimetest"
	"altalune.id/oagent/internal/webhook"
)

const (
	e2eDevice = "dev_V1StGXR8Z5jdHi6B"
	e2eSecret = "whsec_end_to_end"
)

var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}

type sentReply struct{ To, Text, ReplyTo, Key string }

type fakeOpenWA struct {
	mu    sync.Mutex
	srv   *httptest.Server
	sends []sentReply
	reads []string
}

func newFakeOpenWA(t *testing.T) *fakeOpenWA {
	t.Helper()
	f := &fakeOpenWA{}
	base := "/api/v1/orgs/acme/projects/main"
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+base+"/devices/{device}/messages", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ To, Text, ReplyTo string }
		var raw map[string]string
		_ = json.NewDecoder(r.Body).Decode(&raw)
		body.To, body.Text, body.ReplyTo = raw["to"], raw["text"], raw["reply_to"]
		f.mu.Lock()
		f.sends = append(f.sends, sentReply{To: body.To, Text: body.Text, ReplyTo: body.ReplyTo, Key: r.Header.Get("Idempotency-Key")})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"msg_Out0000000000000"}`))
	})
	mux.HandleFunc("GET "+base+"/messages/{id}/media", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBytes)
	})
	mux.HandleFunc("POST "+base+"/messages/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reads = append(f.reads, r.PathValue("id"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenWA) sent() []sentReply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentReply(nil), f.sends...)
}

func (f *fakeOpenWA) marked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reads...)
}

type safeLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *safeLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *safeLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type chStack struct {
	hooks string
	conn  *channel.Connection
	logs  *safeLog
	admin *sql.DB
	sch   string
}

func newChannelStack(t *testing.T, rt *runtimetest.Runtime, wa *fakeOpenWA) *chStack {
	t.Helper()
	logs := &safeLog{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := devLocalConfig()
	cfg.HTTP.BaseURL = "http://oagent.test"
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Runtime = config.RuntimeConfig{Driver: config.RuntimeDriverHTTP, URL: rt.URL(), Token: rt.Token()}
	cfg.Run = config.RunConfig{
		TurnTimeout: 2 * time.Minute, LeaseTTL: 30 * time.Second, Heartbeat: time.Second, DrainTimeout: 5 * time.Second,
		SweepEvery: time.Hour, MaxAttempts: 3, MaxConcurrentPerInstance: 4, MaxConcurrentPerProject: 4, MaxPerMinutePerProject: 100,
	}
	cfg.Channel = config.ChannelConfig{MaxMessageAge: 30 * time.Minute}
	cfg.RateLimit.MaxKeys = 1000
	cfg.RateLimit.Ingest = config.IngestLimitConfig{Enabled: true, PerConnectionPerMinute: 600, Burst: 600, PerIPPerMinute: 600}
	pool, admin, schema := pgtest.MigratedRLS(t)
	cfg.Postgres.Schema = schema
	k := kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false), Log: log}
	svcs := boot.BuildServices(k, cfg)
	lims, err := boot.BuildLimiters(cfg, nil, log)
	require.NoError(t, err)
	runs, err := boot.BuildRun(k, cfg, svcs, lims.Shared(), log)
	require.NoError(t, err)
	svcs.RunStore, svcs.Runs, svcs.RunTurns, svcs.RunKicker, svcs.RuntimeReady = runs.Store, runs.Svc, runs.Runner, runs.Kicker, runs.Ready
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runs.Runner.Run(ctx)
	}()

	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, svcs.OrgStore.Save(tenant.WithOrg(t.Context(), o.ID), o))
	prj, err := svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	in := tenant.Into(t.Context(), tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	provider, err := svcs.ModelProviders.Create(in, prj.ID, modelprovider.Input{
		Name: "fake", Kind: modelprovider.KindOpenAICompatible, BaseURL: rt.ModelBaseURL(),
		Models: []modelprovider.Model{{ID: runtimetest.ModelID, Vision: true}},
	}, "sk-fake")
	require.NoError(t, err)
	a, err := agent.New(o.ID, prj.ID, "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, svcs.AgentStore.Save(in, a))
	require.NoError(t, a.SetDraft(agent.Config{Instructions: "Be brief.", Model: runtimetest.ModelID, ProviderID: provider.ID}, time.Now()))
	v, _, err := a.Publish(nil, uuid.Nil, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, errors.Join(svcs.AgentStore.InsertVersion(in, v), svcs.AgentStore.MarkPublished(in, a), svcs.AgentStore.SaveDraft(in, a)))
	conn, err := svcs.Channels.Create(in, prj.ID, a.ID, channel.Input{
		BaseURL: wa.srv.URL, OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: e2eDevice,
	}, "owk_end_to_end")
	require.NoError(t, err)
	_, err = svcs.Channels.SetSecret(in, conn.ID, e2eSecret, false)
	require.NoError(t, err)

	hooks := httptest.NewServer(boot.BuildIngest(cfg, svcs, db.NewTransactor(pool), lims, log))
	t.Cleanup(hooks.Close)
	t.Cleanup(func() {
		stop()
		<-done
	})
	return &chStack{hooks: hooks.URL, conn: conn, logs: logs, admin: admin, sch: schema}
}

func fixture(t *testing.T, name, msgID, body string, at time.Time) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../channel/openwa/testdata/message_matched_" + name + ".json")
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(raw, &data))
	msg := data["message"].(map[string]any)
	msg["id"], msg["timestamp"] = msgID, at.UTC().Format(time.RFC3339)
	if body != "" {
		msg["body"] = body
	}
	env, err := json.Marshal(map[string]any{
		"id": "evt_" + uuid.NewString(), "type": "message.matched", "api_version": "v1", "created_at": at.UTC().Format(time.RFC3339),
		"tenant": map[string]any{"org_id": uuid.NewString(), "project_id": uuid.NewString(), "project_slug": "main"},
		"data":   data,
	})
	require.NoError(t, err)
	return env
}

func (s *chStack) deliver(t *testing.T, body []byte) {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.hooks+"/hooks/openwa/"+s.conn.PublicID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-Openwa-Event-Type", "message.matched")
	req.Header.Set("X-Openwa-Timestamp", ts)
	req.Header.Set("X-Openwa-Signature", webhook.Sign(e2eSecret, ts, body))
	req.Header.Set("X-Openwa-Delivery-Id", "dlv_"+uuid.NewString())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode, string(raw))
}

var (
	uuidLike   = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	longDigits = regexp.MustCompile(`\d{8,}`)
	jidLike    = regexp.MustCompile(`@(s\.whatsapp\.net|lid|g\.us)`)
	// NOTE: the harness's own container API calls carry 64-hex container ids, not oagent data.
	dockerAPI = regexp.MustCompile(`^(GET|POST|DELETE) /v1\.\d+/`)
)

// SECURITY: a phone, LID or JID never reaches a log line or a span; UUIDs are dropped first, since one can hold 8 digits in a row.
func assertNoPersonal(t *testing.T, where, text string) {
	t.Helper()
	text = uuidLike.ReplaceAllString(text, "<uuid>")
	assert.Empty(t, jidLike.FindAllString(text, -1), "%s holds a JID", where)
	assert.Empty(t, longDigits.FindAllString(text, -1), "%s holds a digit run of 8 or more", where)
}

func TestChannel_AgainstRuntimeContainer(t *testing.T) {
	t.Run("a signed delivery gets one reply, with no phone in logs or spans", func(t *testing.T) {
		spans := tracetest.NewSpanRecorder()
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
		t.Cleanup(func() { otel.SetTracerProvider(prev) })
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Chunks: []string{"Masih ada, ", "**dua** gelas."}, InputTokens: 50, OutputTokens: 5})
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, runtimetest.StartRuntime(t, model), wa)
		s.deliver(t, fixture(t, "text", "msg_Tx1Bq8Lm3Np6Rs9d", "", time.Now()))
		require.Eventually(t, func() bool { return len(wa.sent()) == 1 }, time.Minute, 100*time.Millisecond)
		got := wa.sent()[0]
		assert.Equal(t, "Masih ada, *dua* gelas.", got.Text, "the formatted model reply")
		assert.Equal(t, "628111222333", got.To, "a DM reply goes to the sender's phone")
		assert.Empty(t, got.ReplyTo, "a DM never quotes")
		assert.True(t, strings.HasPrefix(got.Key, "run:") && strings.HasSuffix(got.Key, ":0"), got.Key)
		assert.Equal(t, []string{"msg_Tx1Bq8Lm3Np6Rs9d"}, wa.marked(), "the read receipt")
		time.Sleep(time.Second)
		assert.Len(t, wa.sent(), 1, "one reply")
		assert.Equal(t, 1, model.Calls())

		assertNoPersonal(t, "logs", s.logs.String())
		var ours int
		for _, sp := range spans.Ended() {
			if dockerAPI.MatchString(sp.Name()) {
				continue
			}
			ours++
			var b strings.Builder
			b.WriteString(sp.Name())
			for _, a := range sp.Attributes() {
				b.WriteString(" " + a.Value.String())
			}
			assertNoPersonal(t, "span "+sp.Name(), b.String())
		}
		assert.Positive(t, ours, "the provider calls are traced")
	})

	t.Run("a shuffled burst makes one ordered turn", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Chunks: []string{"Siap."}, InputTokens: 50, OutputTokens: 2})
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, runtimetest.StartRuntime(t, model), wa)
		now := time.Now()
		s.deliver(t, fixture(t, "text", "msg_Burst00000000003", "tiga", now))
		s.deliver(t, fixture(t, "text", "msg_Burst00000000001", "satu", now.Add(-2*time.Second)))
		s.deliver(t, fixture(t, "text", "msg_Burst00000000002", "dua", now.Add(-time.Second)))
		require.Eventually(t, func() bool { return len(wa.sent()) == 1 }, time.Minute, 100*time.Millisecond)
		require.Equal(t, 1, model.Calls(), "the quiet window joins the burst into one turn")
		body := string(model.Requests()[0])
		one, two, three := strings.Index(body, "satu"), strings.Index(body, "dua"), strings.Index(body, "tiga")
		require.True(t, one > 0 && two > 0 && three > 0, body)
		assert.Less(t, one, two, "ordered by WhatsApp time, not arrival")
		assert.Less(t, two, three)
	})

	t.Run("a photo reaches the runtime as an image part", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Chunks: []string{"Struk kopi Rp 45.000."}, InputTokens: 80, OutputTokens: 6})
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, runtimetest.StartRuntime(t, model), wa)
		s.deliver(t, fixture(t, "image", "msg_Im2Cr9Mn4Pq7St0e", "", time.Now()))
		require.Eventually(t, func() bool { return len(wa.sent()) == 1 }, time.Minute, 100*time.Millisecond)
		body := string(model.Requests()[0])
		assert.Contains(t, body, "data:image/jpeg;base64,", "the model got the photo bytes")
		assert.NotContains(t, body, "msg_Im2Cr9Mn4Pq7St0e", "the storage-only id never reaches the model")
		assert.Equal(t, "Struk kopi Rp 45.000.", wa.sent()[0].Text)
	})

	t.Run("a crash mid-turn resumes and replies once", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{Hang: true},
			runtimetest.Reply{Chunks: []string{"Kembali."}, InputTokens: 50, OutputTokens: 2},
			runtimetest.Reply{Chunks: []string{"Lagi."}, InputTokens: 60, OutputTokens: 2},
		)
		rt := runtimetest.StartRuntime(t, model)
		wa := newFakeOpenWA(t)
		s := newChannelStack(t, rt, wa)
		s.deliver(t, fixture(t, "text", "msg_Crash0000000001", "halo", time.Now()))
		select {
		case <-model.Hung():
		case <-time.After(time.Minute):
			t.Fatal("the model never got the turn")
		}
		rt.Kill()
		require.Eventually(t, func() bool {
			var n int
			_ = s.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM `+s.sch+`.runs WHERE status = 'interrupted'`).Scan(&n)
			return n == 1
		}, time.Minute, 100*time.Millisecond, "the cut turn settles interrupted")
		assert.Empty(t, wa.sent(), "an interrupted run sends nothing")
		rt.Restart()
		s.deliver(t, fixture(t, "text", "msg_Crash0000000002", "masih di sana?", time.Now()))
		require.Eventually(t, func() bool { return len(wa.sent()) == 2 }, 2*time.Minute, 200*time.Millisecond)
		time.Sleep(2 * time.Second)
		sends := wa.sent()
		require.Len(t, sends, 2, "the resumed run and the next turn reply once each")
		keys := map[string]bool{}
		for _, r := range sends {
			assert.False(t, keys[r.Key], "no part is sent twice: %s", r.Key)
			keys[r.Key] = true
		}
		assert.Equal(t, []string{"Kembali.", "Lagi."}, []string{sends[0].Text, sends[1].Text})
	})
}
