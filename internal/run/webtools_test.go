package run_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

const (
	webKey    = "sk-brave-SECRET-KEY"
	webSecret = "Bearer SECRET-VALUE"
)

type webFixture struct {
	*svcFixture
	web  *fakes.RunWebTools
	logs *bytes.Buffer
}

func newWebBlock(t *testing.T) *webFixture {
	t.Helper()
	f := &webFixture{svcFixture: newWebSvc(t), logs: &bytes.Buffer{}}
	f.version.Web = run.WebTools{
		Search: true, Fetch: true, HTTP: true,
		FetchAllowDomains: []string{"docs.example.com"}, HTTPHosts: []string{"api.example.com"},
		RequestHeaders: []string{"X-Api-Version"}, TimeoutMs: 20000, MaxCalls: 7,
	}
	f.web = &fakes.RunWebTools{
		Search: run.WebSearchProvider{Kind: "brave", APIKey: webKey, PricePerCallMicros: 5000},
		Opened: []run.WebSecret{{Host: "api.example.com", Header: "authorization", Value: webSecret}},
	}
	f.svc.WithWebTools(f.web).WithLogger(slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return f
}

func TestWebSpec_AllToolsSent(t *testing.T) {
	f := newWebBlock(t)
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	w := f.rt.LastRequest().Web
	require.NotNil(t, w)
	assert.Equal(t, int32(20000), w.TimeoutMs)
	assert.Equal(t, int32(7), w.MaxCallsPerTurn)
	assert.Equal(t, &runtime.WebSearchSpec{Kind: "brave", APIKey: webKey}, w.Search)
	assert.Equal(t, &runtime.WebFetchSpec{AllowDomains: []string{"docs.example.com"}}, w.Fetch)
	assert.Equal(t, &runtime.WebHTTPSpec{
		Hosts: []string{"api.example.com"}, RequestHeaders: []string{"X-Api-Version"},
		Secrets: []runtime.WebSecret{{Host: "api.example.com", Header: "authorization", Value: webSecret}},
	}, w.HTTP)
	assert.Equal(t, [][]string{{"api.example.com"}}, f.web.SecretHosts, "secrets are asked for the allowed hosts only")
}

func TestWebSpec_NoToolNoBlock(t *testing.T) {
	f := newWebBlock(t)
	f.version.Web = run.WebTools{}
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.Nil(t, req.Web)
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"web"`)
	assert.Empty(t, f.web.SecretHosts)
}

func TestWebSpec_SecretsOnlyForAllowedHosts(t *testing.T) {
	f := newWebBlock(t)
	f.web.Opened = append(f.web.Opened, run.WebSecret{Host: "evil.example.com", Header: "authorization", Value: "STOLEN"})
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	raw, err := json.Marshal(f.rt.LastRequest())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "STOLEN")
	assert.NotContains(t, string(raw), "evil.example.com")
}

func TestWebSpec_ListShapes(t *testing.T) {
	f := newWebBlock(t)
	f.version.Web = run.WebTools{Fetch: true, HTTP: true, HTTPHosts: []string{"API.Example.com"}, TimeoutMs: 15000, MaxCalls: 10}
	f.web.Opened = nil
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	raw, err := json.Marshal(f.rt.LastRequest().Web)
	require.NoError(t, err)
	assert.JSONEq(t, `{"timeoutMs":15000,"maxCallsPerTurn":10,"fetch":{"allowDomains":[]},"http":{"hosts":["api.example.com"],"requestHeaders":[],"secrets":[]}}`, string(raw))
}

func TestWebSpec_ProviderMissingOmitsSearch(t *testing.T) {
	for reason, err := range map[string]error{
		"search_provider_missing": run.ErrNoSearchProvider,
		"search_key_unsealable":   run.ErrSearchKeyUnsealable,
		"search_provider_error":   assert.AnError,
	} {
		t.Run(reason, func(t *testing.T) {
			f := newWebBlock(t)
			f.version.Web = run.WebTools{Search: true, TimeoutMs: 15000, MaxCalls: 10}
			f.web.SearchErr = err
			r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			assert.Equal(t, run.StatusCompleted, r.Status, "the turn runs without the tool")
			assert.Nil(t, f.rt.LastRequest().Web, "no tool left, no block")
			assert.Contains(t, f.logs.String(), reason)
			assert.NotContains(t, f.logs.String(), "SECRET")
		})
	}
}

func TestWebSpec_UnsealableSecretDropped(t *testing.T) {
	f := newWebBlock(t)
	f.web.Unsealable = 1
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status)
	require.NotNil(t, f.rt.LastRequest().Web.HTTP, "the tool stays")
	assert.Contains(t, f.logs.String(), "http_secret_unsealable")
	assert.NotContains(t, f.logs.String(), "SECRET")
}

func TestWebSpec_SecretsPortErrorFailsTurn(t *testing.T) {
	f := newWebBlock(t)
	f.web.SecretsErr = assert.AnError
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.NotEqual(t, run.StatusCompleted, r.Status, "a store error never sends http_request without its secrets")
	assert.Empty(t, f.rt.Requests, "the runtime is never called")
}

func TestWebFetch_SendsTheHelperModel(t *testing.T) {
	f := newWebBlock(t)
	f.version.MaxCostCents = 50
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.NotNil(t, req.Agent.HelperModel)
	assert.Equal(t, "gpt-4o-mini", req.Agent.HelperModel.ModelID)
	assert.Equal(t, &runtime.Prices{Input: 150_000, Output: 600_000, CacheRead: 75_000}, req.Limits.Budget.HelperPrices)
}

func TestWebFetch_UnpricedHelperOmitsFetch(t *testing.T) {
	f := newWebBlock(t)
	f.version.MaxCostCents = 50
	f.providers.Models[f.provider]["gpt-4o-mini"] = run.ResolvedModel{BaseURL: "https://llm.example.com/v1", APIKey: "sk-helper"}
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.NotNil(t, req.Web)
	assert.Nil(t, req.Web.Fetch, "a priced run never takes on helper tokens it cannot price")
	assert.Nil(t, req.Agent.HelperModel)
}

func TestPinWebSearch(t *testing.T) {
	f := newWebBlock(t)
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r.Prices.WebSearch)
	assert.Equal(t, int64(5000), *r.Prices.WebSearch, "pinned at run open from the project provider")

	g := newWebBlock(t)
	g.web.SearchErr = run.ErrNoSearchProvider
	r = g.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Nil(t, r.Prices.WebSearch, "no provider, no price")
}

func TestBudget_WebSearchPriceOnlyWithSearch(t *testing.T) {
	price := int64(5000)
	for name, tc := range map[string]struct {
		cents  int32
		search bool
		want   *int64
	}{
		"budget and search": {50, true, &price},
		"budget only":       {50, false, nil},
		"search only":       {0, true, nil},
	} {
		t.Run(name, func(t *testing.T) {
			f := newWebBlock(t)
			f.version.MaxCostCents = tc.cents
			f.version.Web.Search = tc.search
			f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			b := f.rt.LastRequest().Limits.Budget
			if tc.cents == 0 {
				assert.Nil(t, b)
				return
			}
			assert.Equal(t, tc.want, b.WebSearchPrice)
		})
	}
}

func webCall(t *testing.T, method string) []byte {
	t.Helper()
	raw := stream(t, "tool_call")
	raw = bytes.ReplaceAll(raw, []byte(`yasaku__record_expense`), []byte(`http_request`))
	raw = bytes.ReplaceAll(raw, []byte(`{"amount":25000,"account":"cash","note":"lunch"}`), []byte(`{"method":"`+method+`","url":"https://api.example.com/x"}`))
	return bytes.ReplaceAll(raw, []byte(`{\"amount\":25000,\"account\":\"cash\",\"note\":\"lunch\"}`), []byte(`{\"method\":\"`+method+`\",\"url\":\"https://api.example.com/x\"}`))
}

func TestAudit_WebCallsMatchGrants(t *testing.T) {
	f := newWebBlock(t)
	audit := f.withAudit(t)
	f.withApprovals()
	f.execute(t, fakes.RuntimeReply{Stream: webCall(t, "GET")})
	assert.Zero(t, audit.mismatches(t), "a GET http_request is free")

	g := newWebBlock(t)
	gated := g.withAudit(t)
	g.withApprovals()
	g.execute(t, fakes.RuntimeReply{Stream: webCall(t, "POST")})
	assert.Equal(t, int64(1), gated.mismatches(t), "a POST with no consumed approval alerts")
}

func TestWebGrants_MatchSharedApprovalJSON(t *testing.T) {
	raw, err := os.ReadFile("../kernel/webpolicy/testdata/approval.json")
	require.NoError(t, err)
	var shared map[string]struct {
		Mode     string          `json:"mode"`
		FreeWhen json.RawMessage `json:"freeWhen"`
	}
	require.NoError(t, json.Unmarshal(raw, &shared))
	grants := run.WebGrants(runtime.WebSpec{Search: &runtime.WebSearchSpec{}, Fetch: &runtime.WebFetchSpec{}, HTTP: &runtime.WebHTTPSpec{}})
	require.Len(t, grants, len(shared))
	for name, spec := range shared {
		g, ok := grants[name]
		require.True(t, ok, name)
		assert.Equal(t, spec.Mode != "never", g.RequireApproval, name)
		if spec.Mode == "conditional" {
			assert.JSONEq(t, string(spec.FreeWhen), string(g.FreeWhen), name)
		} else {
			assert.Empty(t, g.FreeWhen, name)
		}
	}
	assert.Empty(t, run.WebGrants(runtime.WebSpec{}), "only sent tools get a grant")
}

func TestWebSpec_SentOnResumeLeg(t *testing.T) {
	f := newWebBlock(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.Equal(t, runtime.InputResume, req.Input.Kind)
	require.NotNil(t, req.Web, "an approved POST runs on the resume leg, so the tool must be there")
	assert.NotNil(t, req.Web.HTTP)
}

func TestTurn_KeyNeverLogged(t *testing.T) {
	f := newWebBlock(t)
	f.execute(t, fakes.RuntimeReply{Err: assert.AnError})
	assert.NotContains(t, f.logs.String(), "SECRET")
}

func TestBudget_NoWebSearchPriceWhenSearchIsLeftOutOnALaterTurn(t *testing.T) {
	f := newWebBlock(t)
	f.version.MaxCostCents = 50
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	require.NotNil(t, parked.Prices.WebSearch, "the price was pinned at run open")
	f.web.SearchErr = run.ErrSearchKeyUnsealable
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.Equal(t, runtime.InputResume, req.Input.Kind)
	require.NotNil(t, req.Web)
	assert.Nil(t, req.Web.Search, "the key would not open on the resume turn")
	assert.Nil(t, req.Limits.Budget.WebSearchPrice, "no search sent, no search price")
}

func withWebSearchCalls(t *testing.T, raw []byte, n string) []byte {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	i := bytes.LastIndexByte(trimmed, '\n') + 1
	last := bytes.Replace(trimmed[i:], []byte(`"estimated":false}`), []byte(`"estimated":false,"webSearchCalls":`+n+`}`), 1)
	require.NotEqual(t, trimmed[i:], last, "the terminal carries usage")
	return append(append([]byte(nil), trimmed[:i]...), last...)
}

func TestTerminalUsage_CarriesWebSearchCalls(t *testing.T) {
	f := newWebBlock(t)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)
	r := f.execute(t, fakes.RuntimeReply{Stream: withWebSearchCalls(t, stream(t, "plain_reply"), "2")})
	assert.Equal(t, int64(2), r.Usage.WebSearchCalls)
	micros, priced := run.Cost(r.Usage, r.Prices)
	require.True(t, priced)
	assert.Equal(t, int64(10_000), micros-mustCost(t, run.Usage{InputTokens: r.Usage.InputTokens, OutputTokens: r.Usage.OutputTokens}, r.Prices), "2 calls at 5000")
	require.Len(t, pub.Events(), 1)
	p := events.RunSettledV1(pub.Events()[0].Data.(events.RunCompletedV1))
	assert.Equal(t, int64(2), p.Usage.WebSearchCalls, "the webhook carries the search calls")
	require.NotNil(t, p.CostMicros)
	assert.Equal(t, micros, *p.CostMicros)
}

func mustCost(t *testing.T, u run.Usage, p *run.Prices) int64 {
	t.Helper()
	micros, priced := run.Cost(u, p)
	require.True(t, priced)
	return micros
}

func TestWebSpec_MatchesGolden(t *testing.T) {
	raw, err := os.ReadFile("../kernel/runtime/testdata/turn/web_tools.request.json")
	require.NoError(t, err)
	var golden struct {
		Web json.RawMessage `json:"web"`
	}
	require.NoError(t, json.Unmarshal(raw, &golden))
	require.NotEmpty(t, golden.Web)

	f := newWebBlock(t)
	f.web.Search = run.WebSearchProvider{Kind: "brave", APIKey: "sk-golden-search", PricePerCallMicros: 5000}
	f.web.Opened = []run.WebSecret{{Host: "api.example.com", Header: "authorization", Value: "golden-secret"}}
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	got, err := json.Marshal(f.rt.LastRequest().Web)
	require.NoError(t, err)
	assert.JSONEq(t, string(golden.Web), string(got), "the web block Go builds is the shape the runtime validates")
}

func TestWebSearch_OpensTheKeyOncePerTurn(t *testing.T) {
	f := newWebBlock(t)
	f.version.MaxCostCents = 50
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, f.rt.LastRequest().Web.Search)
	assert.Equal(t, 1, f.web.SearchCalls(), "the price and the key come from one open")
}
