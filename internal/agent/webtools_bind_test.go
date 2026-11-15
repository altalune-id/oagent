package agent_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

func (f *cpGrants) withWebDraft(w agent.Web) {
	f.agent.Draft = agent.Config{
		ProviderID: f.provider, Model: "gpt-4o",
		MCPServers: []agent.MCPGrant{{ServerID: f.server, Tools: []agent.Tool{{Name: "record_expense", RequireApproval: true}}}},
		Web:        w,
	}
}

func (f *cpGrants) asProjectKey(scopes ...string) {
	f.ctx = session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: f.agent.OrgID, ProjectIDs: []uuid.UUID{f.agent.ProjectID},
		Scopes: append([]string{scope.AgentRead, scope.AgentWrite, scope.McpserverRead}, scopes...),
	})
}

// SECURITY: turning on web search or adding an HTTP host spends project credentials, so it needs the whole project and provider:read.
func TestAgentService_WebCredentialsNeedWholeProjectAndProviderRead(t *testing.T) {
	cases := map[string]struct {
		as   func(f *cpGrants)
		web  *agentv1.WebTools
		want string
	}{
		"search, agent-limited key with provider:read": {
			as: func(f *cpGrants) {
				f.asKey([]uuid.UUID{f.agent.ID, f.provider, f.server}, scope.ProviderRead, scope.McpserverRead)
			},
			web: &agentv1.WebTools{Search: true}, want: "web_search_provider",
		},
		"host, agent-limited key with provider:read": {
			as: func(f *cpGrants) {
				f.asKey([]uuid.UUID{f.agent.ID, f.provider, f.server}, scope.ProviderRead, scope.McpserverRead)
			},
			web: &agentv1.WebTools{Http: true, HttpHosts: []string{"api.example.com"}}, want: "http_hosts",
		},
		"search, project key without provider:read": {
			as:  func(f *cpGrants) { f.asProjectKey() },
			web: &agentv1.WebTools{Search: true}, want: "web_search_provider",
		},
		"host, project key without provider:read": {
			as:  func(f *cpGrants) { f.asProjectKey() },
			web: &agentv1.WebTools{Http: true, HttpHosts: []string{"api.example.com"}}, want: "http_hosts",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCPGrants(t)
			f.withWebDraft(agent.Web{})
			tc.as(f)
			c := f.wire()
			c.Web = tc.web
			_, err := f.draft(c)
			assert.Equal(t, tc.want, grantReason(t, err))
			assert.Nil(t, f.store.Saved, "nothing is saved")
		})
	}
}

func TestAgentService_WebCredentialsAlreadyOnTheDraftStay(t *testing.T) {
	f := newCPGrants(t)
	f.withWebDraft(agent.Web{Search: true, HTTP: true, HTTPHosts: []string{"api.example.com", "b.example.com"}})
	f.asKey([]uuid.UUID{f.agent.ID})
	c := f.wire()
	c.Instructions = "Be very brief."
	c.Web = &agentv1.WebTools{Search: true, Http: true, HttpHosts: []string{"api.example.com", "b.example.com"}}
	_, err := f.draft(c)
	require.NoError(t, err, "an agent-limited key edits the agent with the web credentials it already has")

	c.Web = &agentv1.WebTools{Http: true, HttpHosts: []string{"api.example.com"}, Fetch: true, FetchAllowDomains: []string{"docs.example.com"}}
	_, err = f.draft(c)
	require.NoError(t, err, "turning search off, dropping a host or adding fetch domains spends no project credential")
}

func TestAgentService_WebCredentialsBindWithWholeProjectAndProviderRead(t *testing.T) {
	f := newCPGrants(t)
	f.withWebDraft(agent.Web{})
	f.asProjectKey(scope.ProviderRead)
	c := f.wire()
	c.Web = &agentv1.WebTools{Search: true, Http: true, HttpHosts: []string{"api.example.com"}}
	_, err := f.draft(c)
	require.NoError(t, err)
	assert.True(t, f.store.Saved.Draft.Web.Search)
	assert.Equal(t, []string{"api.example.com"}, f.store.Saved.Draft.Web.HTTPHosts)
}
