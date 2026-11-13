//go:build integration

package e2etest

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/testutil/runtimetest"
	"altalune.id/oagent/scope"
)

type dpStack struct {
	url, key, agentPub string
	// second is another instance over the same database, with its own runner.
	second string

	cfg      config.Config
	k        kernel.Kernel
	svcs     *boot.Services
	lims     boot.Limiters
	in       context.Context
	admin    *sql.DB
	sch      string
	org      uuid.UUID
	agentID  uuid.UUID
	provider uuid.UUID
}

func newDPStack(t *testing.T, rt *runtimetest.Runtime) *dpStack {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Runtime = config.RuntimeConfig{Driver: config.RuntimeDriverHTTP, URL: rt.URL(), Token: rt.Token()}
	cfg.Run = config.RunConfig{
		TurnTimeout: 2 * time.Minute, LeaseTTL: 30 * time.Second, Heartbeat: time.Second, DrainTimeout: 5 * time.Second,
		SweepEvery: time.Hour, MaxAttempts: 3, MaxConcurrentPerInstance: 4, MaxConcurrentPerProject: 4, MaxPerMinutePerProject: 100,
	}
	cfg.RateLimit.DataPlane = config.DataPlaneLimitConfig{PerKeyPerMinute: 600, PerIPPerMinute: 600}
	pool, admin, schema := pgtest.MigratedRLS(t)
	cfg.Postgres.Schema = schema
	k := kernel.Kernel{Pool: pool, Reporter: apperror.NewReporter(nil, false)}
	svcs := boot.BuildServices(k, cfg)
	lims, err := boot.BuildLimiters(cfg, nil, testLogger())
	require.NoError(t, err)
	runs, err := boot.BuildRun(k, cfg, svcs, lims.Shared(), testLogger())
	require.NoError(t, err)
	svcs.RunStore, svcs.Runs, svcs.RunTurns, svcs.RunKicker, svcs.RuntimeReady = runs.Store, runs.Svc, runs.Runner, runs.Kicker, runs.Ready
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runs.Runner.Run(ctx)
	}()
	require.Eventually(t, runs.Runner.Running, 10*time.Second, time.Millisecond, "the runner starts")

	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, svcs.OrgStore.Save(tenant.WithOrg(t.Context(), o.ID), o))
	prj, err := svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	in := tenant.Into(t.Context(), tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	provider, err := svcs.ModelProviders.Create(in, prj.ID, modelprovider.Input{
		Name: "fake", Kind: modelprovider.KindOpenAICompatible, BaseURL: rt.ModelBaseURL(),
		Models: []modelprovider.Model{{ID: runtimetest.ModelID}},
	}, "sk-fake")
	require.NoError(t, err)
	a, err := agent.New(o.ID, prj.ID, "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, svcs.AgentStore.Save(in, a))
	require.NoError(t, a.SetDraft(agent.Config{Instructions: "Be brief.", Model: runtimetest.ModelID, ProviderID: provider.ID}, time.Now()))
	v, _, err := a.Publish(nil, uuid.Nil, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, errors.Join(svcs.AgentStore.InsertVersion(in, v), svcs.AgentStore.MarkPublished(in, a), svcs.AgentStore.SaveDraft(in, a)))
	exp := time.Now().Add(time.Hour)
	_, key, err := svcs.APIKeys.Create(t.Context(), apikey.CreateRequest{
		OrgID: o.ID, Kind: apikey.KindOrg, AllProjects: true, Name: "integrator", ExpiresAt: &exp,
		Scopes: []string{scope.RunCreate, scope.RunRead, scope.ApprovalDecide},
	})
	require.NoError(t, err)

	srv := httptest.NewServer(boot.BuildDataPlane(cfg, svcs, lims, k.Reporter.Unexpected, testLogger()))
	t.Cleanup(srv.Close)
	other := *svcs
	otherRuns, err := boot.BuildRun(k, cfg, &other, lims.Shared(), testLogger())
	require.NoError(t, err)
	other.RunStore, other.Runs, other.RunTurns = otherRuns.Store, otherRuns.Svc, otherRuns.Runner
	otherDone := make(chan struct{})
	go func() {
		defer close(otherDone)
		_ = otherRuns.Runner.Run(ctx)
	}()
	require.Eventually(t, otherRuns.Runner.Running, 10*time.Second, time.Millisecond, "the runner starts")
	srv2 := httptest.NewServer(boot.BuildDataPlane(cfg, &other, lims, k.Reporter.Unexpected, testLogger()))
	t.Cleanup(srv2.Close)
	// NOTE: registered last so it runs first: the runners drain, which ends every held stream, before the servers close.
	t.Cleanup(func() {
		stop()
		<-done
		<-otherDone
	})
	return &dpStack{
		url: srv.URL, key: key, agentPub: a.PublicID, second: srv2.URL,
		cfg: cfg, k: k, svcs: svcs, lims: lims, in: in, admin: admin, sch: schema, org: o.ID, agentID: a.ID, provider: provider.ID,
	}
}

func (s *dpStack) do(ctx context.Context, t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	return s.at(ctx, t, s.url, method, path, body)
}

func (s *dpStack) at(ctx context.Context, t *testing.T, base, method, path, body string, headers ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, base+"/api/v1/agents/"+s.agentPub+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(dataplane.UserHeader, "alice")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func (s *dpStack) post(ctx context.Context, t *testing.T, thread, text string) *http.Response {
	t.Helper()
	return s.do(ctx, t, http.MethodPost, "/runs",
		`{"threadId":"`+thread+`","runId":"r","messages":[{"id":"`+uuid.NewString()+`","role":"user","content":"`+text+`"}]}`)
}

type sseEvent struct {
	Type    string `json:"type"`
	RunID   string `json:"runId"`
	Thread  string `json:"threadId"`
	Delta   string `json:"delta"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func readSSE(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	var out []sseEvent
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var e sseEvent
		require.NoError(t, json.Unmarshal([]byte(data), &e))
		out = append(out, e)
	}
	return out
}

func types(events []sseEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func goldenTypes(t *testing.T, name string) []string {
	t.Helper()
	f, err := os.Open("../../dataplane/testdata/sse/" + name + ".sse")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	return types(readSSE(t, f))
}

func (s *dpStack) runStatus(t *testing.T, runID string) string {
	t.Helper()
	resp := s.do(t.Context(), t, http.MethodGet, "/runs/"+runID, "")
	defer func() { _ = resp.Body.Close() }()
	var view struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	return view.Status
}

func TestDataPlane_AgainstRuntimeContainer(t *testing.T) {
	t.Run("a POST streams the turn and a reload reads it back", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Chunks: []string{"Hi, ", "how can I help?"}, InputTokens: 120, OutputTokens: 9})
		s := newDPStack(t, runtimetest.StartRuntime(t, model))
		resp := s.post(t.Context(), t, "conv-1", "hello")
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
		events := readSSE(t, resp.Body)
		assert.Equal(t, goldenTypes(t, "plain_reply"), types(events), "the stream has the golden's shape")
		require.NotEmpty(t, events)
		assert.Equal(t, "conv-1", events[0].Thread, "the client's own thread id")
		runID := events[0].RunID
		require.NotEmpty(t, runID)
		var text strings.Builder
		for _, e := range events {
			text.WriteString(e.Delta)
		}
		assert.Equal(t, "Hi, how can I help?", text.String())
		require.Eventually(t, func() bool { return s.runStatus(t, runID) == "completed" }, 10*time.Second, 50*time.Millisecond)

		page := s.do(t.Context(), t, http.MethodGet, "/threads/conv-1/messages", "")
		defer func() { _ = page.Body.Close() }()
		require.Equal(t, http.StatusOK, page.StatusCode)
		var got struct {
			Messages []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(page.Body).Decode(&got))
		require.Len(t, got.Messages, 2)
		assert.Equal(t, "user", got.Messages[0].Message.Role)
		assert.Equal(t, "Hi, how can I help?", got.Messages[1].Message.Content)
	})

	t.Run("a disconnect still settles the turn", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Chunks: []string{"Still ", "here."}, InputTokens: 10, OutputTokens: 2})
		s := newDPStack(t, runtimetest.StartRuntime(t, model))
		ctx, hangUp := context.WithCancel(t.Context())
		resp := s.post(ctx, t, "conv-2", "hello")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(line, "id: "), line)
		hangUp()
		_ = resp.Body.Close()
		require.Eventually(t, func() bool {
			page := s.do(t.Context(), t, http.MethodGet, "/threads/conv-2/messages", "")
			defer func() { _ = page.Body.Close() }()
			raw, _ := io.ReadAll(page.Body)
			return strings.Contains(string(raw), "Still here.")
		}, 20*time.Second, 100*time.Millisecond, "the turn finished without its client")
		assert.Equal(t, 1, model.Calls())
	})

	t.Run("a crash mid-stream ends interrupted and resumes", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t,
			runtimetest.Reply{Hang: true},
			runtimetest.Reply{Chunks: []string{"Back."}, InputTokens: 50, OutputTokens: 2},
			runtimetest.Reply{Chunks: []string{"Again."}, InputTokens: 60, OutputTokens: 2},
		)
		rt := runtimetest.StartRuntime(t, model)
		s := newDPStack(t, rt)
		resp := s.post(t.Context(), t, "conv-3", "hello")
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		select {
		case <-model.Hung():
		case <-time.After(time.Minute):
			t.Fatal("the model never got the turn")
		}
		rt.Kill()
		events := readSSE(t, resp.Body)
		require.NotEmpty(t, events)
		last := events[len(events)-1]
		assert.Equal(t, "RUN_ERROR", last.Type)
		assert.Equal(t, "interrupted", last.Code)
		assert.Empty(t, last.RunID, "RUN_ERROR carries no runId")
		crashed := events[0].RunID
		assert.Equal(t, "interrupted", s.runStatus(t, crashed))

		rt.Restart()
		again := s.post(t.Context(), t, "conv-3", "are you there")
		defer func() { _ = again.Body.Close() }()
		require.Equal(t, http.StatusOK, again.StatusCode)
		resumed := readSSE(t, again.Body)
		require.NotEmpty(t, resumed)
		assert.Equal(t, crashed, resumed[0].RunID, "the stream follows the resumed run")
		assert.Equal(t, "RUN_FINISHED", resumed[len(resumed)-1].Type)
		require.Eventually(t, func() bool { return s.runStatus(t, crashed) == "completed" }, 20*time.Second, 100*time.Millisecond)
	})

	t.Run("an idempotent retry on another instance never starts a second turn", func(t *testing.T) {
		model := runtimetest.NewFakeModel(t, runtimetest.Reply{Hang: true})
		s := newDPStack(t, runtimetest.StartRuntime(t, model))
		body := `{"threadId":"conv-4","messages":[{"id":"m1","role":"user","content":"hello"}]}`
		first := s.at(t.Context(), t, s.url, http.MethodPost, "/runs", body, dataplane.IdempotencyHeader, "retry-1")
		defer func() { _ = first.Body.Close() }()
		require.Equal(t, http.StatusOK, first.StatusCode)
		select {
		case <-model.Hung():
		case <-time.After(time.Minute):
			t.Fatal("the model never got the turn")
		}
		retry := s.at(t.Context(), t, s.second, http.MethodPost, "/runs", body, dataplane.IdempotencyHeader, "retry-1")
		defer func() { _ = retry.Body.Close() }()
		assert.Equal(t, http.StatusConflict, retry.StatusCode, "the same request, still running elsewhere")
		var env struct {
			Code string            `json:"code"`
			Meta map[string]string `json:"meta"`
		}
		require.NoError(t, json.NewDecoder(retry.Body).Decode(&env))
		assert.Equal(t, "RUN007", env.Code)
		assert.NotEmpty(t, env.Meta["run"], "it names the run already going")
		changed := s.at(t.Context(), t, s.second, http.MethodPost, "/runs",
			`{"threadId":"conv-4","messages":[{"id":"m1","role":"user","content":"something else"}]}`, dataplane.IdempotencyHeader, "retry-1")
		defer func() { _ = changed.Body.Close() }()
		assert.Equal(t, http.StatusUnprocessableEntity, changed.StatusCode, "the same key with another body")
		assert.Equal(t, 1, model.Calls(), "one turn")
	})
}
