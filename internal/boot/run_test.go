package boot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

func TestRunAgents_MapsThePinnedVersion(t *testing.T) {
	ag := &agent.Agent{ID: uuid.New(), PublicID: "agent-pub"}
	provider, server, memStore, stt := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	v := &agent.Version{ID: uuid.New(), AgentID: ag.ID, Version: 4, Config: agent.Config{
		Instructions: "be brief", Model: "gpt-4o", Temperature: 0.3, MaxSteps: 7, MaxCostCents: 50, ProviderID: provider,
		HelperModel: "mini", MemoryStoreID: memStore, TranscriptionModel: agent.TranscriptionModel{ProviderID: stt, Model: "whisper-1"},
		MCPServers: []agent.MCPGrant{{ServerID: server, Tools: []agent.Tool{{Name: "a"}, {Name: "b", RequireApproval: true}}}},
	}}
	store := &fakes.AgentStore{Agent: ag, Published: []*agent.Version{v}}
	a := runAgents{store: store}
	ctx := context.Background()

	got, err := a.Version(ctx, v.ID)
	require.NoError(t, err)
	assert.Equal(t, &run.AgentVersion{
		ID: v.ID, AgentID: ag.ID, AgentPublicID: "agent-pub", Version: 4, Instructions: "be brief", Model: "gpt-4o",
		Temperature: 0.3, MaxSteps: 7, MaxCostCents: 50, ProviderID: provider, HelperModel: "mini", MemoryStoreID: memStore,
		TranscriptionModel: run.TranscriptionModel{ProviderID: stt, Model: "whisper-1"},
		MCPServers:         []run.MCPGrant{{ServerID: server, Tools: []run.ToolGrant{{Name: "a"}, {Name: "b", RequireApproval: true}}}},
		Knowledge:          run.KnowledgeTuning{MaxBytes: knowledge.RecommendedMaxTokens * 4},
	}, got)
	latest, err := a.Latest(ctx, ag.ID)
	require.NoError(t, err)
	assert.Equal(t, got, latest)

	legacy := &agent.Version{ID: uuid.New(), AgentID: ag.ID, Config: agent.Config{Model: "x"}}
	store.Published = []*agent.Version{legacy}
	got, err = a.Latest(ctx, ag.ID)
	require.NoError(t, err)
	assert.True(t, got.Legacy, "a version without a provider is legacy")
	assert.Equal(t, int32(agent.RecommendedMaxSteps), got.MaxSteps, "an unset step cap runs with the Go recommended value, never 0")

	store.Published = nil
	got, err = a.Latest(ctx, ag.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "an unpublished agent has no version")
	_, err = a.Version(ctx, uuid.New())
	assert.True(t, agent.IsNotFoundError(err))
}

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

func TestRunAgents_DraftRefusalIsExpected(t *testing.T) {
	ag := &agent.Agent{ID: uuid.New(), PublicID: "agent-pub", Draft: agent.Config{Model: "gpt-4o"}}
	store := &fakes.AgentStore{Agent: ag}
	a := runAgents{store: store, agents: agent.NewService(store, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected)}

	_, err := a.Draft(t.Context(), ag.ID)
	require.ErrorIs(t, err, run.ErrDraftRefused, "a draft refused like a publish")
	assert.True(t, agent.IsNeedsRepublishError(err), "the refusal stays readable")
	assert.True(t, run.IsExpectedError(err))

	store.Err = errors.New("db down")
	_, err = a.Draft(t.Context(), ag.ID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, run.ErrDraftRefused, "a failure that is not a refusal is still reported")

	store.Err, store.Agent = nil, nil
	_, err = a.Draft(t.Context(), ag.ID)
	assert.True(t, agent.IsNotFoundError(err), "a deleted agent keeps its old path")
	assert.NotErrorIs(t, err, run.ErrDraftRefused, "a deleted agent's Try message never waits as refused")
}

func TestRunProviders_ResolveWithPrices(t *testing.T) {
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	svc := modelprovider.NewService(&fakes.ModelProviderStore{}, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithSealer(testSealer(t))
	p, err := svc.Create(ctx, uuid.New(), modelprovider.Input{
		Name: "p", Kind: modelprovider.KindOpenAICompatible, BaseURL: "https://llm.example.com/v1",
		Models: []modelprovider.Model{
			{ID: "priced", Prices: &modelprovider.Prices{InputMicros: 1, OutputMicros: 2, CacheReadMicros: 3, CacheWriteMicros: 4}},
			{ID: "free"},
			{ID: "whisper-1", Transcription: true, Prices: &modelprovider.Prices{AudioMinuteMicros: 6_000}},
		},
	}, "sk-live")
	require.NoError(t, err)
	r := runProviders{providers: svc}

	stt, err := r.ResolveModel(ctx, p.ID, "whisper-1")
	require.NoError(t, err)
	require.NotNil(t, stt.Prices)
	assert.Equal(t, new(int64(6_000)), stt.Prices.Transcription, "a transcription model carries its price per audio minute")

	m, err := r.ResolveModel(ctx, p.ID, "priced")
	require.NoError(t, err)
	assert.Equal(t, "https://llm.example.com/v1", m.BaseURL)
	assert.Equal(t, "sk-live", m.APIKey)
	assert.Empty(t, m.Provider, "an OpenAI-compatible provider sends no provider")
	require.NotNil(t, m.Prices)
	assert.Equal(t, run.Prices{Input: new(int64(1)), Output: new(int64(2)), CacheRead: new(int64(3)), CacheWrite: new(int64(4))}, *m.Prices,
		"a chat model leaves the transcription price unset")

	m, err = r.ResolveModel(ctx, p.ID, "free")
	require.NoError(t, err)
	assert.Nil(t, m.Prices, "an unpriced model has no snapshot")
	_, err = r.ResolveModel(ctx, p.ID, "nope")
	assert.Error(t, err)
}

func TestWireProvider_EveryKindMaps(t *testing.T) {
	want := map[modelprovider.Kind]string{
		modelprovider.KindOpenAICompatible: "",
		modelprovider.KindAnthropic:        runtime.ProviderAnthropic,
	}
	for _, k := range modelprovider.Kinds() {
		got, err := wireProvider(k)
		require.NoError(t, err, k)
		w, ok := want[k]
		require.True(t, ok, "kind %q has no expected wire value", k)
		assert.Equal(t, w, got, k)
	}
	_, err := wireProvider("bogus")
	assert.Error(t, err)
}

func TestRunProviders_ResolveCarriesTheProvider(t *testing.T) {
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	svc := modelprovider.NewService(&fakes.ModelProviderStore{}, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithSealer(testSealer(t))
	p, err := svc.Create(ctx, uuid.New(), modelprovider.Input{
		Name: "claude", Kind: modelprovider.KindAnthropic, BaseURL: "https://api.anthropic.com",
		Models: []modelprovider.Model{{ID: "claude-sonnet-5-5"}},
	}, "sk-live")
	require.NoError(t, err)

	m, err := runProviders{providers: svc}.ResolveModel(ctx, p.ID, "claude-sonnet-5-5")
	require.NoError(t, err)
	assert.Equal(t, runtime.ProviderAnthropic, m.Provider)
}

func TestRunServers_OpenTheCredential(t *testing.T) {
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	svc := mcpserver.NewService(&fakes.MCPServerStore{}, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected).
		WithSealer(testSealer(t))
	srv, err := svc.Create(ctx, uuid.New(), mcpserver.Input{
		Name: "yasaku", URL: "https://yasaku.example.com/mcp", AuthKind: mcpserver.AuthHeader, AuthHeader: "X-Key", TimeoutMS: 4000,
	}, "tok-live")
	require.NoError(t, err)
	got, err := runServers{servers: svc}.Open(ctx, srv.ID)
	require.NoError(t, err)
	assert.Equal(t, run.OpenedServer{
		Name: "yasaku", URL: "https://yasaku.example.com/mcp", AuthKind: "header", AuthHeader: "X-Key", AuthValue: "tok-live", TimeoutMS: 4000,
	}, got)
	_, err = runServers{servers: svc}.Open(ctx, uuid.New())
	assert.Error(t, err)
}

func TestBuildRuntime(t *testing.T) {
	rt, err := buildRuntime(config.RuntimeConfig{})
	require.NoError(t, err)
	assert.Nil(t, rt, "no url, no runtime")
	rt, err = buildRuntime(config.RuntimeConfig{Driver: config.RuntimeDriverHTTP, URL: "http://runtime:5160", Token: "t"})
	require.NoError(t, err)
	assert.NotNil(t, rt)
	_, err = buildRuntime(config.RuntimeConfig{Driver: "grpc", URL: "http://runtime:5160"})
	assert.Error(t, err)
	_, err = buildRuntime(config.RuntimeConfig{Driver: config.RuntimeDriverHTTP, URL: "::bad"})
	assert.Error(t, err)
}

func TestRunTranscriber_CallsTheProvider(t *testing.T) {
	var auth, model string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, model = r.Header.Get("Authorization"), r.FormValue("model")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"text":"halo"}`))
	}))
	t.Cleanup(srv.Close)
	m := run.ResolvedModel{BaseURL: srv.URL + "/v1", APIKey: "sk-stt"}

	text, err := runTranscriber{allowPrivateHosts: true}.Transcribe(t.Context(), m, "whisper-1", []byte("OggS"), "audio/ogg")
	require.NoError(t, err)
	assert.Equal(t, "halo", text)
	assert.Equal(t, "Bearer sk-stt", auth)
	assert.Equal(t, "whisper-1", model)

	status = http.StatusTooManyRequests
	_, err = runTranscriber{allowPrivateHosts: true}.Transcribe(t.Context(), m, "whisper-1", []byte("OggS"), "audio/ogg")
	assert.ErrorIs(t, err, run.ErrTranscribeRateLimited)

	_, err = runTranscriber{}.Transcribe(t.Context(), m, "whisper-1", []byte("OggS"), "audio/ogg")
	require.Error(t, err, "a private host needs the opt-in")
	assert.NotErrorIs(t, err, run.ErrTranscribeRateLimited)

	m.Provider = runtime.ProviderAnthropic
	_, err = runTranscriber{allowPrivateHosts: true}.Transcribe(t.Context(), m, "whisper-1", []byte("OggS"), "audio/ogg")
	assert.Error(t, err, "only an OpenAI-compatible provider transcribes")
}
