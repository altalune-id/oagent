package agent_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/agent"
)

type webSearchPort struct {
	has bool
	err error
}

func (p webSearchPort) HasSearchProvider(context.Context, uuid.UUID) (bool, error) {
	return p.has, p.err
}

func publishWeb(t *testing.T, port agent.WebSearch, web agent.Web, edit func(*agent.Config)) (*grantsFixture, error) {
	t.Helper()
	f := newGrantsFixture(t)
	if port != nil {
		f.svc = f.svc.WithWebSearch(port)
	}
	cfg := f.config()
	cfg.Web = web
	if edit != nil {
		edit(&cfg)
	}
	f.agent.Draft = cfg
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	return f, err
}

func TestPublish_WebSearchWithoutProvider(t *testing.T) {
	_, err := publishWeb(t, webSearchPort{has: false}, agent.Web{Search: true}, nil)
	assert.Equal(t, "web_search_provider", grantReason(t, err))
	_, err = publishWeb(t, nil, agent.Web{Search: true}, nil)
	assert.Equal(t, "web_search_provider", grantReason(t, err), "no port fails closed")
	_, err = publishWeb(t, webSearchPort{has: true}, agent.Web{Search: true}, nil)
	require.NoError(t, err)
	_, err = publishWeb(t, webSearchPort{err: assert.AnError}, agent.Web{Search: true}, nil)
	require.ErrorIs(t, err, assert.AnError)
}

func TestDraftRun_WebSearchWithoutProviderRefused(t *testing.T) {
	f := newGrantsFixture(t)
	f.svc = f.svc.WithWebSearch(webSearchPort{has: false})
	f.agent.Draft = f.config()
	f.agent.Draft.Web = agent.Web{Search: true}
	_, err := f.svc.DraftVersion(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "web_search_provider", grantReason(t, err), "Try it refuses the same draft")
}

func TestPublish_HTTPWithoutHosts(t *testing.T) {
	_, err := publishWeb(t, nil, agent.Web{HTTP: true}, nil)
	assert.Equal(t, "http_hosts", grantReason(t, err))
	_, err = publishWeb(t, nil, agent.Web{HTTP: true, HTTPHosts: []string{"api.example.com"}}, nil)
	require.NoError(t, err)
}

func TestPublish_FetchNeedsADomainList(t *testing.T) {
	for name, edit := range map[string]func(*agent.Config){
		"with a granted MCP tool": func(c *agent.Config) {},
		"reading nothing private": func(c *agent.Config) { c.MCPServers = nil },
	} {
		_, err := publishWeb(t, nil, agent.Web{Fetch: true}, edit)
		assert.Equal(t, "fetch_allow_domains", grantReason(t, err), name)
		_, err = publishWeb(t, nil, agent.Web{Fetch: true, FetchAllowDomains: []string{"docs.example.com"}}, edit)
		require.NoError(t, err, name)
	}
}

func TestPublish_WebMakesUnattendedToolRefused(t *testing.T) {
	for name, web := range map[string]agent.Web{
		"search": {Search: true},
		"fetch":  {Fetch: true, FetchAllowDomains: []string{"docs.example.com"}},
		"http":   {HTTP: true, HTTPHosts: []string{"api.example.com"}},
	} {
		_, err := publishWeb(t, webSearchPort{has: true}, web, func(c *agent.Config) {
			c.MCPServers[0].Tools = []agent.Tool{{Name: "send"}}
		})
		assert.Equal(t, "yasaku__send", refusedTool(t, err), name)
	}
}
