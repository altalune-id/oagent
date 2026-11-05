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
	"altalune.id/oagent/internal/testutil/fakes"
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
