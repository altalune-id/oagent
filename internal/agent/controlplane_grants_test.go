package agent_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

type cpGrants struct {
	*grantsFixture
	refs        *fakes.PublicIDs
	cp          *agent.AgentService
	ctx         context.Context
	providerPub string
	serverPub   string
}

func newCPGrants(t *testing.T) *cpGrants {
	t.Helper()
	f := &cpGrants{grantsFixture: newGrantsFixture(t), refs: fakes.NewPublicIDs()}
	f.refs.Org(f.agent.OrgID)
	f.refs.Project(f.agent.ProjectID, f.agent.OrgID)
	f.providerPub = f.refs.Provider(f.provider, f.agent.ProjectID)
	f.serverPub = f.refs.MCPServer(f.server, f.agent.ProjectID)
	f.cp = agent.NewControlPlane(f.svc, f.refs)
	f.ctx = fakes.OperatorContext(context.Background())
	f.store.Agent.PublicID = f.agent.PublicID
	return f
}

func (f *cpGrants) wire() *agentv1.AgentConfig {
	return &agentv1.AgentConfig{
		Instructions: "Be brief.", ProviderId: f.providerPub, Model: "gpt-4o", HelperModel: "mini", MaxCostCents: 100,
		McpServers: []*agentv1.McpGrant{{ServerId: f.serverPub, Tools: []*agentv1.AgentTool{{Name: "record_expense", RequireApproval: true}}}},
	}
}

func (f *cpGrants) draft(cfg *agentv1.AgentConfig) (*agentv1.Agent, error) {
	resp, err := f.cp.UpdateAgentDraft(f.ctx, connect.NewRequest(&agentv1.UpdateAgentDraftRequest{Id: f.agent.PublicID, Config: cfg}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetAgent(), nil
}

func TestAgentService_DraftMapsGrantPublicIDs(t *testing.T) {
	f := newCPGrants(t)
	out, err := f.draft(f.wire())
	require.NoError(t, err)
	assert.Equal(t, f.provider, f.store.Saved.Draft.ProviderID)
	assert.Equal(t, f.server, f.store.Saved.Draft.MCPServers[0].ServerID)

	d := out.GetDraft()
	assert.Equal(t, f.providerPub, d.GetProviderId())
	assert.False(t, d.GetProviderMissing())
	assert.Equal(t, "mini", d.GetHelperModel())
	require.Len(t, d.GetMcpServers(), 1)
	assert.Equal(t, f.serverPub, d.GetMcpServers()[0].GetServerId())
	assert.False(t, d.GetMcpServers()[0].GetMissing())
	assert.Equal(t, "record_expense", d.GetMcpServers()[0].GetTools()[0].GetName())

	raw, err := protojson.Marshal(out)
	require.NoError(t, err)
	for _, id := range []uuid.UUID{f.provider, f.server, f.agent.ID, f.agent.OrgID, f.agent.ProjectID} {
		assert.NotContains(t, string(raw), id.String(), "no UUID on the wire")
	}
}

func TestAgentService_DraftRefusesForeignOrUnknownReferences(t *testing.T) {
	f := newCPGrants(t)
	otherProject := uuid.New()
	foreignProvider := f.refs.Provider(uuid.New(), otherProject)
	foreignServer := f.refs.MCPServer(uuid.New(), otherProject)
	for want, edits := range map[string][]func(*agentv1.AgentConfig){
		"provider": {
			func(c *agentv1.AgentConfig) { c.ProviderId = foreignProvider },
			func(c *agentv1.AgentConfig) { c.ProviderId = "unknownunknownunknown" },
			func(c *agentv1.AgentConfig) { c.ProviderId = f.provider.String() },
		},
		"server": {
			func(c *agentv1.AgentConfig) { c.McpServers[0].ServerId = foreignServer },
			func(c *agentv1.AgentConfig) { c.McpServers[0].ServerId = "unknownunknownunknown" },
		},
	} {
		for _, edit := range edits {
			c := f.wire()
			edit(c)
			_, err := f.draft(c)
			assert.Equal(t, want, grantReason(t, err))
		}
	}
	assert.Nil(t, f.store.Saved)
}

func TestAgentService_DanglingReferencesRenderMissing(t *testing.T) {
	f := newCPGrants(t)
	gone, goneServer := uuid.New(), uuid.New()
	f.agent.Draft = agent.Config{ProviderID: gone, Model: "gpt-4o", MCPServers: []agent.MCPGrant{{ServerID: goneServer, Tools: []agent.Tool{{Name: "t"}}}}}
	resp, err := f.cp.GetAgent(f.ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: f.agent.PublicID}))
	require.NoError(t, err, "a dangling reference never fails GetAgent")
	d := resp.Msg.GetAgent().GetDraft()
	assert.Empty(t, d.GetProviderId())
	assert.True(t, d.GetProviderMissing())
	require.Len(t, d.GetMcpServers(), 1)
	assert.Empty(t, d.GetMcpServers()[0].GetServerId())
	assert.True(t, d.GetMcpServers()[0].GetMissing())
	raw, err := protojson.Marshal(resp.Msg)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), gone.String())
	assert.NotContains(t, string(raw), goneServer.String())
}

func TestAgentService_LegacyRendersWithoutAProvider(t *testing.T) {
	f := newCPGrants(t)
	f.agent.Draft = agent.Config{Model: "gpt-4o", Tools: []agent.Tool{{Name: "x.y"}}}
	resp, err := f.cp.GetAgent(f.ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: f.agent.PublicID}))
	require.NoError(t, err)
	d := resp.Msg.GetAgent().GetDraft()
	assert.Empty(t, d.GetProviderId())
	assert.False(t, d.GetProviderMissing(), "no provider is not a missing one")
	assert.Equal(t, "x.y", d.GetTools()[0].GetName())

	_, err = f.cp.PublishAgent(f.ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: f.agent.PublicID}))
	assert.True(t, agent.IsNeedsRepublishError(err), "got %v", err)
}

func TestAgentService_PublishRendersTheGrants(t *testing.T) {
	f := newCPGrants(t)
	_, err := f.draft(f.wire())
	require.NoError(t, err)
	resp, err := f.cp.PublishAgent(f.ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: f.agent.PublicID}))
	require.NoError(t, err)
	v := resp.Msg.GetVersion().GetConfig()
	assert.Equal(t, f.providerPub, v.GetProviderId())
	assert.Equal(t, f.serverPub, v.GetMcpServers()[0].GetServerId())
}

func TestAgentService_DraftWithADeletedProvider(t *testing.T) {
	f := newCPGrants(t)
	_, err := f.draft(f.wire())
	require.NoError(t, err)
	f.refs.Remove(f.provider)
	delete(f.grants.Providers, f.provider)

	resp, err := f.cp.GetAgent(f.ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: f.agent.PublicID}))
	require.NoError(t, err, "the draft still loads")
	assert.True(t, resp.Msg.GetAgent().GetDraft().GetProviderMissing())
	assert.Empty(t, resp.Msg.GetAgent().GetDraft().GetProviderId())

	_, err = f.cp.PublishAgent(f.ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: f.agent.PublicID}))
	assert.Equal(t, "provider", grantReason(t, err))
	assert.Empty(t, f.store.Published)
}

func (f *cpGrants) asKey(resources []uuid.UUID, scopes ...string) {
	f.ctx = session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: f.agent.OrgID,
		ProjectIDs: []uuid.UUID{f.agent.ProjectID}, ResourceIDs: resources,
		Scopes: append([]string{scope.AgentRead, scope.AgentWrite}, scopes...),
	})
}

func TestAgentService_DraftNeedsReachAndScopeForANewReference(t *testing.T) {
	cases := map[string]struct {
		resources func(f *cpGrants) []uuid.UUID
		scopes    []string
		edit      func(f *cpGrants, c *agentv1.AgentConfig)
		want      string
	}{
		"provider without provider:read": {
			resources: func(f *cpGrants) []uuid.UUID { return []uuid.UUID{f.agent.ID, f.provider, f.server} },
			scopes:    []string{scope.McpserverRead},
			want:      "provider",
		},
		"provider outside the key's resources": {
			resources: func(f *cpGrants) []uuid.UUID { return []uuid.UUID{f.agent.ID, f.server} },
			scopes:    []string{scope.ProviderRead, scope.McpserverRead},
			want:      "provider",
		},
		"server without mcpserver:read": {
			resources: func(f *cpGrants) []uuid.UUID { return []uuid.UUID{f.agent.ID, f.provider, f.server} },
			scopes:    []string{scope.ProviderRead},
			want:      "server",
		},
		"server outside the key's resources": {
			resources: func(f *cpGrants) []uuid.UUID { return []uuid.UUID{f.agent.ID, f.provider} },
			scopes:    []string{scope.ProviderRead, scope.McpserverRead},
			want:      "server",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newCPGrants(t)
			f.asKey(tc.resources(f), tc.scopes...)
			_, err := f.draft(f.wire())
			assert.Equal(t, tc.want, grantReason(t, err))
			assert.Nil(t, f.store.Saved)
		})
	}
}

func TestAgentService_DraftKeepsReferencesAlreadyOnTheDraft(t *testing.T) {
	f := newCPGrants(t)
	f.agent.Draft = agent.Config{
		ProviderID: f.provider, Model: "gpt-4o",
		MCPServers: []agent.MCPGrant{{ServerID: f.server, Tools: []agent.Tool{{Name: "record_expense"}}}},
	}
	f.asKey([]uuid.UUID{f.agent.ID})
	c := f.wire()
	c.Instructions = "Be very brief."
	_, err := f.draft(c)
	require.NoError(t, err, "a key limited to the agent edits it with the references it already has")
	assert.Equal(t, "Be very brief.", f.store.Saved.Draft.Instructions)

	c.McpServers = nil
	_, err = f.draft(c)
	require.NoError(t, err, "removing a reference needs nothing")
}

func TestAgentService_DraftBindsWithReachAndScope(t *testing.T) {
	f := newCPGrants(t)
	f.asKey([]uuid.UUID{f.agent.ID, f.provider, f.server}, scope.ProviderRead, scope.McpserverRead)
	_, err := f.draft(f.wire())
	require.NoError(t, err)
	assert.Equal(t, f.provider, f.store.Saved.Draft.ProviderID)

	f.asKey(nil, scope.ProviderRead, scope.McpserverRead)
	_, err = f.draft(f.wire())
	require.NoError(t, err, "a key over the whole project reaches every resource")
}

func (f *cpGrants) boundServer() {
	f.agent.Draft = agent.Config{
		ProviderID: f.provider, Model: "gpt-4o",
		MCPServers: []agent.MCPGrant{{ServerID: f.server, Tools: []agent.Tool{
			{Name: "record_expense", RequireApproval: true}, {Name: "list_wallets"},
		}}},
	}
}

func (f *cpGrants) withTools(tools ...*agentv1.AgentTool) *agentv1.AgentConfig {
	c := f.wire()
	c.McpServers[0].Tools = tools
	return c
}

func TestAgentService_DraftWideningABoundServerNeedsReachAndScope(t *testing.T) {
	for name, tools := range map[string][]*agentv1.AgentTool{
		"a new tool":   {{Name: "record_expense", RequireApproval: true}, {Name: "list_wallets"}, {Name: "delete_tx"}},
		"approval off": {{Name: "record_expense"}, {Name: "list_wallets"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCPGrants(t)
			f.boundServer()
			f.asKey([]uuid.UUID{f.agent.ID})
			_, err := f.draft(f.withTools(tools...))
			assert.Equal(t, "server", grantReason(t, err))
			assert.Nil(t, f.store.Saved)

			f.asKey([]uuid.UUID{f.agent.ID, f.server}, scope.McpserverRead)
			_, err = f.draft(f.withTools(tools...))
			require.NoError(t, err, "with reach on the server and mcpserver:read the widening is allowed")
		})
	}
}

func TestAgentService_DraftNarrowingABoundServerIsFree(t *testing.T) {
	for name, tools := range map[string][]*agentv1.AgentTool{
		"a tool removed": {{Name: "record_expense", RequireApproval: true}},
		"approval on":    {{Name: "record_expense", RequireApproval: true}, {Name: "list_wallets", RequireApproval: true}},
		"unchanged":      {{Name: "record_expense", RequireApproval: true}, {Name: "list_wallets"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCPGrants(t)
			f.boundServer()
			f.asKey([]uuid.UUID{f.agent.ID})
			_, err := f.draft(f.withTools(tools...))
			require.NoError(t, err)
		})
	}
}
