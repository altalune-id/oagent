//go:build integration

package run_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestWeb_SettlePersistsSearchCallsAndPrice(t *testing.T) {
	p := newPgRunner(t)
	provider := uuid.New()
	version := &run.AgentVersion{
		ID: p.s.version, AgentID: p.s.agent, Version: 1, Model: "gpt-4o", ProviderID: provider,
		Web: run.WebTools{Search: true, TimeoutMs: 15000, MaxCalls: 10},
	}
	p.svc.WithAgents(&fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{version.ID: version}}).
		WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{
			provider: {"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test", Prices: &run.Prices{Input: new(int64(1)), Output: new(int64(1)), CacheRead: new(int64(0)), CacheWrite: new(int64(0))}}},
		}}).
		WithWebTools(&fakes.RunWebTools{Search: run.WebSearchProvider{Kind: "brave", APIKey: "k", PricePerCallMicros: 5000}})
	ch := p.chat(t, p.s, "dm")
	thread := p.arrive(t, ch, "wamid-1", "find it", time.Now())
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: withWebSearchCalls(t, stream(t, "plain_reply"), "2")})
	require.NoError(t, err)

	got, err := p.store.RunByID(in(p.s.org), r.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.Usage.WebSearchCalls)
	require.NotNil(t, got.Prices.WebSearch)
	assert.Equal(t, int64(5000), *got.Prices.WebSearch, "the pinned price is stored with the run")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE id = $1 AND web_search_calls = 2 AND web_search_price_micros = 5000 AND cost_micros >= 10000`, r.ID))
}
