package run_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newWebSvc(t *testing.T) *svcFixture {
	t.Helper()
	f := newSvc(t)
	f.version.HelperModel = "gpt-4o-mini"
	f.version.Web = run.WebTools{Fetch: true, TimeoutMs: 15000, MaxCalls: 10}
	f.providers.Models[f.provider]["gpt-4o-mini"] = run.ResolvedModel{BaseURL: "https://llm.example.com/v1", APIKey: "sk-helper", Prices: &run.Prices{
		Input: new(int64(150_000)), Output: new(int64(600_000)), CacheRead: new(int64(75_000)), CacheWrite: new(int64(0)),
	}}
	f.say(t, "look it up")
	return f
}

func TestWebFetch_PinsHelperWithoutMemory(t *testing.T) {
	f := newWebSvc(t)
	f.version.MaxCostCents = 50
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r.Prices)
	require.NotNil(t, r.Prices.HelperInput, "web_fetch pins the helper's prices with the run, without memory")
	assert.Equal(t, int64(150_000), *r.Prices.HelperInput)
}

func TestWebFetch_NoSeparateHelperPinsTheMainPrices(t *testing.T) {
	f := newWebSvc(t)
	f.version.HelperModel = ""
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r.Prices.HelperInput)
	assert.Equal(t, int64(2_500_000), *r.Prices.HelperInput)
}

func TestWebFetch_UnpricedHelperPinsNone(t *testing.T) {
	f := newWebSvc(t)
	f.version.MaxCostCents = 50
	f.providers.Models[f.provider]["gpt-4o-mini"] = run.ResolvedModel{BaseURL: "https://llm.example.com/v1", APIKey: "sk-helper"}
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Nil(t, r.Prices.HelperInput, "a priced run never takes on helper tokens it cannot price")
}

func TestWebOff_PinsNoHelper(t *testing.T) {
	f := newWebSvc(t)
	f.version.Web = run.WebTools{}
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Nil(t, r.Prices.HelperInput)
}
