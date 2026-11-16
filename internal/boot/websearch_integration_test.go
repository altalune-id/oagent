//go:build integration

package boot

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/webtool"
	"altalune.id/oagent/scope"
)

func TestPublish_WebSearchFollowsTheProjectProvider(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	p := g.provider(t, "llm")
	a := g.agentUsing(t, "searcher", agent.Config{ProviderID: p.ID, Model: "m", Web: agent.Web{Search: true}})

	_, _, _, err := g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	e, ok := errors.AsType[*agent.InvalidGrantError](err)
	require.True(t, ok, "no provider yet: %v", err)
	assert.Equal(t, agent.GrantWebSearchProvider, e.Reason)

	_, err = f.svcs.WebTools.SetProvider(g.ctx, g.projectID, webtool.KindBrave, new("sk-search"), 0)
	require.NoError(t, err)
	_, _, _, err = g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	require.NoError(t, err)

	require.NoError(t, f.svcs.WebTools.DeleteProvider(g.ctx, g.projectID))
	_, err = g.agents.UpdateDraft(g.ctx, a.ID, agent.Config{ProviderID: p.ID, Model: "m", Instructions: "x", Web: agent.Web{Search: true}})
	require.NoError(t, err, "a draft saves without a provider")
	_, _, _, err = g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	e, ok = errors.AsType[*agent.InvalidGrantError](err)
	require.True(t, ok, "provider deleted: %v", err)
	assert.Equal(t, agent.GrantWebSearchProvider, e.Reason)
}

// SECURITY: the MCP draft tool goes through the same web credential bind rule as the API.
func TestMCP_AgentScopedKeyCannotBorrowWebCredentials(t *testing.T) {
	f := newAPIFixture(t)
	home := newGrantScope(t, f)
	a := home.agentUsing(t, "scoped-web", agent.Config{Instructions: "Be brief."})
	call := func(key string, web map[string]any) string {
		raw, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
				"name": "agent_update_draft",
				"arguments": map[string]any{"id": a.PublicID, "config": map[string]any{
					"instructions": "Be brief.", "web": web,
				}},
			},
		})
		require.NoError(t, err)
		status, _, body := f.mcpCall(t, key, string(raw))
		require.Equal(t, http.StatusOK, status, string(body))
		return string(body)
	}
	scoped := f.mintKey(t, apikey.CreateRequest{
		OrgID: home.orgID, Kind: apikey.KindProject, ProjectID: home.projectID,
		Scopes: []string{scope.AgentRead, scope.AgentWrite, scope.ProviderRead}, ResourceIDs: []uuid.UUID{a.ID},
	})
	for _, web := range []map[string]any{{"search": true}, {"http": true, "httpHosts": []string{"api.example.com"}}} {
		refused := call(scoped, web)
		assert.Contains(t, refused, `"isError":true`, "%v", web)
		assert.Contains(t, refused, string(apperror.CodeAgentInvalidGrant), "%v", web)
	}
	whole := f.mintKey(t, apikey.CreateRequest{
		OrgID: home.orgID, Kind: apikey.KindProject, ProjectID: home.projectID,
		Scopes: []string{scope.AgentRead, scope.AgentWrite, scope.ProviderRead},
	})
	assert.NotContains(t, call(whole, map[string]any{"search": true, "http": true, "httpHosts": []string{"api.example.com"}}), `"isError":true`)
}
