package dataplane_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

const (
	projectKey  = "oak_project"
	orgKey      = "oak_org"
	decideKey   = "oak_decide"
	personalKey = "oak_personal"
	readOnlyKey = "oak_readonly"
	otherKey    = "oak_other"
)

type keys map[string]session.Principal

func (k keys) Authenticate(_ context.Context, c authn.Credential) (session.Principal, error) {
	p, ok := k[c.Token]
	if !ok {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	return p, nil
}

type agents map[string]dataplane.AgentRef

func (a agents) Resolve(_ context.Context, id string) (dataplane.AgentRef, bool, error) {
	r, ok := a[id]
	return r, ok, nil
}

type names struct{}

func (names) UserPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return "U" + id.String()[:8], nil
}

type endUsers struct {
	mu  sync.Mutex
	ids map[string]uuid.UUID
}

func (e *endUsers) UpsertAPIUser(_ context.Context, project uuid.UUID, value string) (uuid.UUID, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ids == nil {
		e.ids = map[string]uuid.UUID{}
	}
	k := project.String() + "/" + value
	if id, ok := e.ids[k]; ok {
		return id, nil
	}
	e.ids[k] = uuid.New()
	return e.ids[k], nil
}

func (e *endUsers) FindAPIUser(_ context.Context, project uuid.UUID, value string) (uuid.UUID, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	id, ok := e.ids[project.String()+"/"+value]
	return id, ok, nil
}

type plane struct {
	srv            *httptest.Server
	store          *fakes.RunStore
	rt             *fakes.Runtime
	runner         *run.Runner
	svc            *run.Service
	eu             *endUsers
	agents         *fakes.RunAgents
	handler        *dataplane.Handler
	org, project   uuid.UUID
	agent, version uuid.UUID
	runtimeReady   bool
}

func runtimeStream(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "kernel", "runtime", "testdata", "turn", name+".stream.ndjson"))
	require.NoError(t, err)
	return raw
}

func newPlane(t *testing.T, maxConcurrent int, opts ...func(*dataplane.Deps)) *plane {
	t.Helper()
	p := &plane{
		store: &fakes.RunStore{}, rt: &fakes.Runtime{}, org: uuid.New(), project: uuid.New(),
		agent: uuid.New(), version: uuid.New(), runtimeReady: true, eu: &endUsers{},
	}
	provider := uuid.New()
	p.agents = &fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{p.version: {
		ID: p.version, AgentID: p.agent, AgentPublicID: "AGENTpub", Version: 1, Model: "gpt-4o", ProviderID: provider,
	}}}
	p.svc = run.NewService(p.store, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithRuntime(p.rt).
		WithAgents(p.agents).
		WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{provider: {
			"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test"},
		}}}).
		WithMCPServers(&fakes.RunMCPServers{}).
		WithSnapshots(&fakes.SnapshotStore{})
	p.runner = run.NewRunner(p.svc, p.store, run.RunnerOptions{
		MaxConcurrent: maxConcurrent, Heartbeat: 20 * time.Millisecond, LeaseTTL: 2 * time.Hour, DrainTimeout: time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.runner.Run(ctx)
	}()
	require.Eventually(t, p.runner.Running, 10*time.Second, time.Millisecond, "the runner starts")
	t.Cleanup(func() {
		cancel()
		<-done
	})
	keyFor := func(scopes []string, all bool, owner uuid.UUID, projects ...uuid.UUID) session.Principal {
		return session.Principal{
			KeyID: uuid.New(), Source: session.SourceAPIKey, Scopes: scopes, ActiveOrgID: p.org,
			AllProjects: all, ProjectIDs: projects, OwnerID: owner,
		}
	}
	deps := dataplane.Deps{
		Keys: keys{
			projectKey:  keyFor([]string{scope.RunCreate, scope.RunRead}, false, uuid.Nil, p.project),
			orgKey:      keyFor([]string{"run:*"}, true, uuid.Nil),
			personalKey: keyFor([]string{"*"}, true, uuid.New()),
			readOnlyKey: keyFor([]string{scope.RunRead}, true, uuid.Nil),
			otherKey:    keyFor([]string{"run:*"}, false, uuid.Nil, uuid.New()),
			decideKey:   keyFor([]string{"run:*", scope.ApprovalDecide}, true, uuid.Nil),
		},
		KeyPrefix: "oak_",
		Agents: agents{
			"AGENTpub":   {ID: p.agent, OrgID: p.org, ProjectID: p.project},
			"FOREIGNpub": {ID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New()},
			"DRAFTpub":   {ID: uuid.New(), OrgID: p.org, ProjectID: p.project, Draft: true},
		},
		Names: names{}, EndUsers: p.eu, Store: p.store, Runner: p.runner, Canceller: p.svc,
		RuntimeReady: func() bool { return p.runtimeReady },
		Unexpected:   apperror.NewReporter(nil, false).Unexpected,
	}
	for _, o := range opts {
		o(&deps)
	}
	h := dataplane.New(deps)
	p.handler = h
	mux := http.NewServeMux()
	h.Register(mux, "")
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func body(thread, text string, extra ...string) string {
	var b strings.Builder
	b.WriteString(`{"threadId":"` + thread + `","messages":[{"id":"m1","role":"user","content":"` + text + `"}]`)
	for _, e := range extra {
		b.WriteString("," + e)
	}
	return b.String() + "}"
}

type result struct {
	StatusCode int
	Header     http.Header
	Body       string
}

func (p *plane) request(t *testing.T, ctx context.Context, key, agent, payload string, headers ...string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.srv.URL+"/api/v1/agents/"+agent+"/runs", strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return req
}

func (p *plane) post(t *testing.T, key, agent, payload string, headers ...string) result {
	t.Helper()
	resp, err := p.srv.Client().Do(p.request(t, t.Context(), key, agent, payload, headers...))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header, Body: string(raw)}
}

func errorCode(t *testing.T, resp result) (apperror.Code, map[string]string) {
	t.Helper()
	var env struct {
		Code      apperror.Code     `json:"code"`
		Message   string            `json:"message"`
		RequestID string            `json:"request_id"`
		Meta      map[string]string `json:"meta"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &env))
	assert.NotEmpty(t, env.RequestID)
	assert.NotEmpty(t, env.Message)
	return env.Code, env.Meta
}

func (p *plane) settled(t *testing.T) *run.Run {
	t.Helper()
	require.Eventually(t, func() bool {
		runs := p.store.AllRuns()
		return len(runs) == 1 && runs[0].Status.Terminal() && p.store.LeaseOwner(firstThread(p)) == ""
	}, 5*time.Second, 10*time.Millisecond)
	r := p.store.AllRuns()[0]
	return &r
}

func firstThread(p *plane) uuid.UUID {
	for _, r := range p.store.AllRuns() {
		return r.ThreadID
	}
	return uuid.Nil
}

func TestPOSTRuns_StreamsATurn(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}}
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))
	assert.Equal(t, string(readGolden(t, "plain_reply.sse")), resp.Body, "the stream is the golden's bytes")
	r := p.settled(t)
	assert.Equal(t, run.StatusCompleted, r.Status)
}

func TestPOSTRuns_StockClientBodyPasses(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}}
	stock := body(clientThread, "hello", `"runId":"client-run-1"`, `"parentRunId":null`, `"state":{}`, `"tools":[]`,
		`"context":[]`, `"forwardedProps":{}`, `"protocolVersion":"1.0"`, `"resume":null`)
	resp := p.post(t, projectKey, "AGENTpub", stock)
	require.Equal(t, http.StatusOK, resp.StatusCode, "a stock HttpAgent 1.0.3 body is accepted")
}

func TestPOSTRuns_ForwardedPropsAndToolsIgnored(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}}
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello",
		`"tools":[{"name":"steal","description":"x","parameters":{}}]`, `"forwardedProps":{"user":"admin"}`, `"context":[{"description":"d","value":"v"}]`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	req := p.rt.LastRequest()
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	for _, never := range []string{"steal", "admin", `"description":"d"`} {
		assert.NotContains(t, string(raw), never, "the runtime request carries nothing the client sent but its text")
	}
	assert.JSONEq(t, `{"id":"m1","role":"user","content":"hello"}`, string(req.Input.Message))
	assert.Empty(t, req.MCPServers)
}

func TestPOSTRuns_PersonalKeyWithUserIs403(t *testing.T) {
	p := newPlane(t, 4)
	resp := p.post(t, personalKey, "AGENTpub", body(clientThread, "hello"), dataplane.UserHeader, "customer-42")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeForbidden, code)
	assert.Empty(t, p.store.Messages)
}

func TestPOSTRuns_NoRunCreateIs403(t *testing.T) {
	p := newPlane(t, 4)
	resp := p.post(t, readOnlyKey, "AGENTpub", body(clientThread, "hello"))
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_, meta := errorCode(t, resp)
	assert.Equal(t, scope.RunCreate, meta["permission"])
}

func TestPOSTRuns_OtherProjectAgentIs404(t *testing.T) {
	p := newPlane(t, 4)
	for name, c := range map[string]struct{ key, agent string }{
		"key of another project": {otherKey, "AGENTpub"},
		"agent of another org":   {orgKey, "FOREIGNpub"},
		"unknown agent":          {orgKey, "NOPE"},
	} {
		resp := p.post(t, c.key, c.agent, body(clientThread, "hello"))
		require.Equal(t, http.StatusNotFound, resp.StatusCode, name)
		code, _ := errorCode(t, resp)
		assert.Equal(t, apperror.CodeAgentNotFound, code, "%s: never forbidden, so nothing leaks", name)
	}
	resp := p.post(t, "oak_unknown", "AGENTpub", body(clientThread, "hello"))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPOSTRuns_UnknownFieldIs400(t *testing.T) {
	p := newPlane(t, 4)
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello", `"user":"admin"`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	code, meta := errorCode(t, resp)
	assert.Equal(t, apperror.CodeValidation, code)
	assert.Equal(t, "user", meta["field"], "the field path is named, never its value")
}

func TestPOSTRuns_LastMessageNotUserIs400(t *testing.T) {
	p := newPlane(t, 4)
	for name, payload := range map[string]string{
		"assistant last": `{"threadId":"t1","messages":[{"id":"m","role":"assistant","content":"hi"}]}`,
		"no messages":    `{"threadId":"t1","messages":[]}`,
		"bad thread id":  `{"threadId":"has space","messages":[{"id":"m","role":"user","content":"hi"}]}`,
		"image content":  `{"threadId":"t1","messages":[{"id":"m","role":"user","content":[{"type":"image","url":"x"}]}]}`,
	} {
		resp := p.post(t, projectKey, "AGENTpub", payload)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
	}
	big := body(clientThread, strings.Repeat("x", 70<<10))
	resp := p.post(t, projectKey, "AGENTpub", big)
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	assert.Empty(t, p.store.Messages)
}

func TestPOSTRuns_OtherActorOnBoundThreadIs404(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}}
	resp := p.post(t, orgKey, "AGENTpub", body(clientThread, "hello"), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	p.settled(t)
	resp = p.post(t, orgKey, "AGENTpub", body(clientThread, "let me in"), dataplane.UserHeader, "bob")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeThreadNotFound, code, "another actor's thread is simply not found")
}

func TestPOSTRuns_409Race(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: cut(runtimeStream(t, "plain_reply")), Block: true}}
	first, err := p.srv.Client().Do(p.request(t, t.Context(), projectKey, "AGENTpub", body(clientThread, "hello")))
	require.NoError(t, err)
	defer func() { _ = first.Body.Close() }()
	require.Equal(t, http.StatusOK, first.StatusCode)
	second := p.post(t, projectKey, "AGENTpub", body(clientThread, "again"))
	require.Equal(t, http.StatusConflict, second.StatusCode, "one live stream per thread")
	code, _ := errorCode(t, second)
	assert.Equal(t, apperror.CodeRunInProgress, code)
	assert.Len(t, p.store.Messages, 1, "the second message was never inserted")
}

func cut(raw []byte) []byte {
	trimmed := strings.TrimSpace(string(raw))
	return []byte(trimmed[:strings.LastIndexByte(trimmed, '\n')+1])
}

func TestPOSTRuns_RunnerFullIs503NothingWritten(t *testing.T) {
	p := newPlane(t, 1)
	slot, ok := p.runner.Reserve()
	require.True(t, ok)
	defer slot.Release()
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"))
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeRunRuntimeUnavailable, code)
	assert.Empty(t, p.store.Messages)
}

func TestPOSTRuns_NoRuntimeIs503NothingWritten(t *testing.T) {
	p := newPlane(t, 4)
	p.runtimeReady = false
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"))
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Empty(t, p.store.Messages)
}

func TestPOSTRuns_ProjectBusyIs429NothingWritten(t *testing.T) {
	p := newPlane(t, 1)
	p.store.ClaimRefused = true
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"))
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeRunRateLimited, code)
	assert.Empty(t, p.store.Messages)
	slot, ok := p.runner.Reserve()
	assert.True(t, ok, "the reserved slot came back")
	slot.Release()
}

func TestPOSTRuns_DisconnectStillSettles(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "tool_call")}}
	ctx, cancel := context.WithCancel(t.Context())
	resp, err := p.srv.Client().Do(p.request(t, ctx, projectKey, "AGENTpub", body(clientThread, "record lunch")))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cancel()
	_ = resp.Body.Close()
	r := p.settled(t)
	assert.Equal(t, run.StatusCompleted, r.Status, "the client left; the run did not")
}

func TestDataPlane_NoCORSHeaders(t *testing.T) {
	p := newPlane(t, 4)
	for _, method := range []string{http.MethodOptions, http.MethodPost} {
		req, err := http.NewRequestWithContext(t.Context(), method, p.srv.URL+"/api/v1/agents/AGENTpub/runs", strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Origin", "https://evil.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		resp, err := p.srv.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		for name := range resp.Header {
			assert.False(t, strings.HasPrefix(name, "Access-Control-"), "%s: %s", method, name)
		}
	}
}

func TestPOSTRuns_IdempotencyKeyGuardsRetries(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: cut(runtimeStream(t, "plain_reply")), Block: true}}
	first, err := p.srv.Client().Do(p.request(t, t.Context(), projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-1"))
	require.NoError(t, err)
	defer func() { _ = first.Body.Close() }()
	require.Equal(t, http.StatusOK, first.StatusCode)

	same := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-1")
	require.Equal(t, http.StatusConflict, same.StatusCode, "the same request while its run is live")
	code, _ := errorCode(t, same)
	assert.Equal(t, apperror.CodeRunInProgress, code)

	other := p.post(t, projectKey, "AGENTpub", body(clientThread, "something else"), dataplane.IdempotencyHeader, "k-1")
	require.Equal(t, http.StatusUnprocessableEntity, other.StatusCode, "the same key with another body")
	code, _ = errorCode(t, other)
	assert.Equal(t, apperror.CodeRunIdempotencyConflict, code)
	assert.Len(t, p.store.Messages, 1)

	bad := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "has space")
	assert.Equal(t, http.StatusBadRequest, bad.StatusCode)
}

func TestPOSTRuns_ActorIdentity(t *testing.T) {
	p := newPlane(t, 4)
	eu := &endUsers{}
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}, {Stream: runtimeStream(t, "plain_reply")}}
	h := dataplane.New(dataplane.Deps{
		Keys:      keys{personalKey: {KeyID: uuid.New(), Source: session.SourceAPIKey, Scopes: []string{"*"}, ActiveOrgID: p.org, AllProjects: true, OwnerID: uuid.New()}},
		KeyPrefix: "oak_", Agents: agents{"AGENTpub": {ID: p.agent, OrgID: p.org, ProjectID: p.project}},
		Names: names{}, EndUsers: eu, Store: p.store, Runner: p.runner, Unexpected: apperror.NewReporter(nil, false).Unexpected,
	})
	mux := http.NewServeMux()
	h.Register(mux, "")
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/agents/AGENTpub/runs", strings.NewReader(body("personal-thread", "hi")))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+personalKey)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var keysSeen []string
	for k := range eu.ids {
		keysSeen = append(keysSeen, k)
	}
	require.Len(t, keysSeen, 1)
	assert.Contains(t, keysSeen[0], "/person:U", "a personal key speaks as its owner")

	invalidUser := p.post(t, orgKey, "AGENTpub", body(clientThread, "hi"), dataplane.UserHeader, "has space")
	assert.Equal(t, http.StatusBadRequest, invalidUser.StatusCode)
}

func TestPOSTRuns_BodyShapes(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply")}}
	notJSON := p.post(t, projectKey, "AGENTpub", `{"threadId":`)
	assert.Equal(t, http.StatusBadRequest, notJSON.StatusCode)
	parts := `{"threadId":"parts","messages":[{"id":"m","role":"user","content":[{"type":"text","text":"hel"},{"type":"text","text":"lo"}]}]}`
	resp := p.post(t, projectKey, "AGENTpub", parts)
	require.Equal(t, http.StatusOK, resp.StatusCode, "text parts are joined")
	assert.Contains(t, string(p.rt.LastRequest().Input.Message), `"content":"hello"`)
}

type brokenNames struct{}

func (brokenNames) UserPublicID(context.Context, uuid.UUID) (string, error) {
	return "", errors.New("db down")
}

func TestPOSTRuns_UnexpectedErrorIsGEN900(t *testing.T) {
	p := newPlane(t, 4)
	h := dataplane.New(dataplane.Deps{
		Keys: keys{projectKey: {
			KeyID: uuid.New(), Source: session.SourceAPIKey, Scopes: []string{"*"}, ActiveOrgID: p.org, AllProjects: true, OwnerID: uuid.New(),
		}},
		KeyPrefix: "oak_", Agents: agents{"AGENTpub": {ID: p.agent, OrgID: p.org, ProjectID: p.project}},
		Names: brokenNames{}, EndUsers: &endUsers{}, Store: p.store, Runner: p.runner, Unexpected: apperror.NewReporter(nil, true).Unexpected,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/agents/AGENTpub/runs", strings.NewReader(body("t", "hi")))
	req.Header.Set("Authorization", "Bearer "+projectKey)
	mux := http.NewServeMux()
	h.Register(mux, "")
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"GEN900"`)
	assert.NotContains(t, rec.Body.String(), "db down", "the cause stays server-side")
}

func TestPOSTRuns_RunHeldRefusalThenRetryIsADuplicate(t *testing.T) {
	var stopped *run.Runner
	p := newPlane(t, 4, func(d *dataplane.Deps) {
		stopped = run.NewRunner(d.Canceller.(*run.Service), d.Store, run.RunnerOptions{MaxConcurrent: 4, Heartbeat: time.Hour, LeaseTTL: time.Hour})
		d.Runner = stopped
	})
	first := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-refused")
	require.Equal(t, http.StatusServiceUnavailable, first.StatusCode, "a runner that is not running refuses after the claim committed")
	assert.NotEmpty(t, first.Header.Get("Retry-After"))
	code, _ := errorCode(t, first)
	assert.Equal(t, apperror.CodeRunRuntimeUnavailable, code)
	require.Len(t, p.store.Messages, 1, "the message stays for the run sweep")
	for id := range p.store.Threads {
		assert.Empty(t, p.store.LeaseOwner(id), "the lease is released so another instance can take it")
	}

	retry := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-refused")
	assert.Equal(t, http.StatusConflict, retry.StatusCode, "the retry finds the stored request")
	retryCode, _ := errorCode(t, retry)
	assert.Equal(t, apperror.CodeRunInProgress, retryCode)
	assert.Len(t, p.store.Messages, 1, "never a second message")
	assert.Empty(t, p.rt.Requests, "no turn ran")
	slot, ok := stopped.Reserve()
	require.True(t, ok, "every slot came back")
	slot.Release()
}

func TestPOSTRuns_LongTurnOutlivesTheReadTimeout(t *testing.T) {
	p := newPlane(t, 4)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: runtimeStream(t, "plain_reply"), Delay: 600 * time.Millisecond}}
	srv := httptest.NewUnstartedServer(p.srv.Config.Handler)
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/agents/AGENTpub/runs", strings.NewReader(body(clientThread, "hello")))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+projectKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 1, resp.ProtoMajor, "HTTP/1.1, where the read deadline ends the request context")
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"type":"RUN_FINISHED"`, "a turn longer than the read timeout still reaches its terminal")
}

func (p *plane) written() (threads, users int) {
	p.eu.mu.Lock()
	defer p.eu.mu.Unlock()
	return len(p.store.Threads), len(p.eu.ids)
}

type refuseAll struct{}

func (refuseAll) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	return ratelimit.Result{Allowed: false, RetryAfter: time.Second}, nil
}

func (refuseAll) Reset(context.Context, string) error { return nil }

func TestPOSTRuns_RefusalsWriteNoRows(t *testing.T) {
	for name, c := range map[string]struct {
		setup  func(p *plane)
		opts   []func(*dataplane.Deps)
		agent  string
		status int
		code   apperror.Code
	}{
		"no runtime":  {setup: func(p *plane) { p.runtimeReady = false }, agent: "AGENTpub", status: http.StatusServiceUnavailable, code: apperror.CodeRunRuntimeUnavailable},
		"runner full": {setup: func(p *plane) { _, _ = p.runner.Reserve() }, agent: "AGENTpub", status: http.StatusServiceUnavailable, code: apperror.CodeRunRuntimeUnavailable},
		"project rate": {
			setup: func(*plane) {}, opts: []func(*dataplane.Deps){func(d *dataplane.Deps) { d.Rate = run.RateCap{Limiter: refuseAll{}, PerMinute: 1} }},
			agent: "AGENTpub", status: http.StatusTooManyRequests, code: apperror.CodeRunRateLimited,
		},
		"unpublished agent": {setup: func(*plane) {}, agent: "DRAFTpub", status: http.StatusConflict, code: apperror.CodeAgentNeedsRepublish},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPlane(t, 1, c.opts...)
			c.setup(p)
			resp := p.post(t, projectKey, c.agent, body(clientThread, "hello"), dataplane.UserHeader, "customer-1")
			require.Equal(t, c.status, resp.StatusCode, resp.Body)
			code, _ := errorCode(t, resp)
			assert.Equal(t, c.code, code)
			threads, users := p.written()
			assert.Zero(t, threads, "no thread row")
			assert.Zero(t, users, "no end user row")
			assert.Empty(t, p.store.Messages)
		})
	}
}

func TestPOSTRuns_IdempotentRetryIsFreeOfChecks(t *testing.T) {
	p := newPlane(t, 1, func(d *dataplane.Deps) { d.Rate = run.RateCap{Limiter: memory.New(100), PerMinute: 1} })
	first := p.settledTurn(t, projectKey, dataplane.IdempotencyHeader, "k-free")
	slot, ok := p.runner.Reserve()
	require.True(t, ok)
	defer slot.Release()
	p.runtimeReady = false
	again := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-free")
	require.Equal(t, http.StatusOK, again.StatusCode, "the replay needs no runtime, slot or quota: %s", again.Body)
	assert.Contains(t, again.Body, first.PublicID)
	other := p.post(t, orgKey, "AGENTpub", body(clientThread, "hello"), dataplane.IdempotencyHeader, "k-free")
	assert.Equal(t, http.StatusNotFound, other.StatusCode, "another actor's thread stays hidden on the early lookup")
}

type brokenAgents struct{}

func (brokenAgents) Resolve(context.Context, string) (dataplane.AgentRef, bool, error) {
	return dataplane.AgentRef{}, false, errors.New("db down")
}

func TestAuthorize_OnlyAMissingAgentIs404(t *testing.T) {
	p := newPlane(t, 4, func(d *dataplane.Deps) {
		d.Agents = brokenAgents{}
		d.Unexpected = apperror.NewReporter(nil, true).Unexpected
	})
	resp := p.post(t, projectKey, "AGENTpub", body(clientThread, "hello"))
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "a lookup failure is not a missing agent")
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeUnexpected, code)
	assert.NotContains(t, resp.Body, "db down")
	missing := newPlane(t, 4).post(t, projectKey, "NOSUCHpub", body(clientThread, "hello"))
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)
}
