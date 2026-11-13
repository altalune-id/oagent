package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newAgentServiceClient(t *testing.T, store agent.Store, refs agent.Refs) agentv1connect.AgentServiceClient {
	t.Helper()
	_, handler := agentv1connect.NewAgentServiceHandler(agent.NewControlPlane(agent.NewService(store, fakes.Transactor{}, nil), refs),
		connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return agentv1connect.NewAgentServiceClient(http.DefaultClient, ts.URL)
}

func grantsFor(a *agent.Agent, refs *fakes.PublicIDs, models ...string) (*fakes.AgentGrants, uuid.UUID, string) {
	provider := uuid.New()
	refs.Provider(provider, a.ProjectID)
	g := &fakes.AgentGrants{Providers: map[uuid.UUID]fakes.GrantProvider{provider: {ProjectID: a.ProjectID}}}
	for _, m := range models {
		p := g.Providers[provider]
		p.Models = append(p.Models, agent.ModelRef{ID: m, Priced: true})
		g.Providers[provider] = p
	}
	pub, _, _ := refs.ProviderPublicID(context.Background(), provider)
	return g, provider, pub
}

func newGrantedClient(t *testing.T, store agent.Store, refs agent.Refs, g *fakes.AgentGrants) agentv1connect.AgentServiceClient {
	t.Helper()
	svc := agent.NewService(store, fakes.Transactor{}, nil).WithProviders(g).WithMCPServers(g).WithLocker(g).WithKnowledge(g)
	_, handler := agentv1connect.NewAgentServiceHandler(agent.NewControlPlane(svc, refs), connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return agentv1connect.NewAgentServiceClient(http.DefaultClient, ts.URL)
}

func registeredAgent(t *testing.T, refs *fakes.PublicIDs, slug string) (a *agent.Agent, orgPub, projectPub string) {
	t.Helper()
	orgID, projectID := uuid.New(), uuid.New()
	a, err := agent.New(orgID, projectID, slug, "Acme", "", uuid.New())
	require.NoError(t, err)
	return a, refs.Org(orgID), refs.Project(projectID, orgID)
}

func TestAgentService_CreateAgent(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID, projectID := uuid.New(), uuid.New()
	orgPub, projectPub := refs.Org(orgID), refs.Project(projectID, orgID)
	store := &fakes.AgentStore{}
	c := newAgentServiceClient(t, store, refs)

	resp, err := c.CreateAgent(context.Background(), connect.NewRequest(&agentv1.CreateAgentRequest{
		OrgId: orgPub, ProjectId: projectPub, Slug: "acme", Name: "Acme", Description: "d",
	}))
	require.NoError(t, err)
	require.NotNil(t, store.Saved)
	assert.Equal(t, orgID, store.Saved.OrgID)
	assert.Equal(t, projectID, store.Saved.ProjectID)
	assert.Equal(t, "acme", resp.Msg.GetAgent().GetSlug())
	assert.Equal(t, store.Saved.PublicID, resp.Msg.GetAgent().GetId())
	assert.Equal(t, orgPub, resp.Msg.GetAgent().GetOrgId())
	assert.Equal(t, projectPub, resp.Msg.GetAgent().GetProjectId())
}

func TestAgentService_CreateAgent_UnknownScope(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID, projectID := uuid.New(), uuid.New()
	orgPub, projectPub := refs.Org(orgID), refs.Project(projectID, orgID)
	store := &fakes.AgentStore{}
	c := newAgentServiceClient(t, store, refs)

	for name, req := range map[string]*agentv1.CreateAgentRequest{
		"no org":       {ProjectId: projectPub, Name: "Acme"},
		"no project":   {OrgId: orgPub, Name: "Acme"},
		"uuid org":     {OrgId: orgID.String(), ProjectId: projectPub, Name: "Acme"},
		"uuid project": {OrgId: orgPub, ProjectId: projectID.String(), Name: "Acme"},
		"org as prj":   {OrgId: orgPub, ProjectId: orgPub, Name: "Acme"},
	} {
		_, err := c.CreateAgent(context.Background(), connect.NewRequest(req))
		assert.ErrorContains(t, err, "not found", name)
	}
	assert.Nil(t, store.Saved)
}

func TestAgentService_GetAgent(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, orgPub, projectPub := registeredAgent(t, refs, "acme")
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a}, refs)

	resp, err := c.GetAgent(context.Background(), connect.NewRequest(&agentv1.GetAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetAgent().GetSlug())
	assert.Equal(t, a.PublicID, resp.Msg.GetAgent().GetId())
	assert.Equal(t, orgPub, resp.Msg.GetAgent().GetOrgId())
	assert.Equal(t, projectPub, resp.Msg.GetAgent().GetProjectId())
}

func TestAgentService_GetAgent_UnknownID(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "acme")
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a}, refs)

	for _, id := range []string{"not-a-public-id", a.ID.String()} {
		_, err := c.GetAgent(context.Background(), connect.NewRequest(&agentv1.GetAgentRequest{Id: id}))
		assert.ErrorContains(t, err, "not found", id)
	}
}

func TestAgentService_ListAgents(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID, projectID := uuid.New(), uuid.New()
	orgPub, projectPub := refs.Org(orgID), refs.Project(projectID, orgID)
	a1, err := agent.New(orgID, projectID, "aaa", "A", "", uuid.New())
	require.NoError(t, err)
	a2, err := agent.New(orgID, projectID, "bbb", "B", "", uuid.New())
	require.NoError(t, err)
	c := newAgentServiceClient(t, &fakes.AgentStore{Agents: []*agent.Agent{a1, a2}}, refs)

	resp, err := c.ListAgents(context.Background(), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: projectPub}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetAgents(), 2)
	assert.Equal(t, a1.PublicID, resp.Msg.GetAgents()[0].GetId())
	assert.Equal(t, orgPub, resp.Msg.GetAgents()[0].GetOrgId())
	assert.Equal(t, projectPub, resp.Msg.GetAgents()[0].GetProjectId())
}

func TestAgentService_ListAgents_UnknownProject(t *testing.T) {
	refs := fakes.NewPublicIDs()
	projectID := uuid.New()
	refs.Project(projectID, uuid.New())
	c := newAgentServiceClient(t, &fakes.AgentStore{}, refs)

	for _, id := range []string{"bad", projectID.String()} {
		_, err := c.ListAgents(context.Background(), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: id}))
		assert.ErrorContains(t, err, "not found", id)
	}
}

func TestAgentService_UpdateAgent(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "acme")
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a}, refs)

	resp, err := c.UpdateAgent(context.Background(), connect.NewRequest(&agentv1.UpdateAgentRequest{Id: a.PublicID, Name: "Acme Two", Description: "x"}))
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", resp.Msg.GetAgent().GetName())
	assert.Equal(t, a.PublicID, resp.Msg.GetAgent().GetId())
}

func TestAgentService_DeleteAgent(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "bot")
	store := &fakes.AgentStore{Agent: a}
	c := newAgentServiceClient(t, store, refs)

	_, err := c.DeleteAgent(context.Background(), connect.NewRequest(&agentv1.DeleteAgentRequest{Id: a.ID.String()}))
	assert.ErrorContains(t, err, "not found")
	assert.Equal(t, uuid.Nil, store.Deleted)

	_, err = c.DeleteAgent(context.Background(), connect.NewRequest(&agentv1.DeleteAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, a.ID, store.Deleted)
}

func TestAgentService_DraftPublishAndVersions(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "acme")
	store := &fakes.AgentStore{Agent: a}
	g, server := grantsWithServer(a, refs)
	providerPub := ""
	for id := range g.Providers {
		providerPub, _, _ = refs.ProviderPublicID(context.Background(), id)
	}
	c := newGrantedClient(t, store, refs, g)
	ctx := context.Background()
	cfg := &agentv1.AgentConfig{
		Instructions: "Be brief.", ProviderId: providerPub, Model: "anthropic/claude-sonnet-5-5", Temperature: 0.5, MaxSteps: 8, MaxCostCents: 300,
		McpServers: []*agentv1.McpGrant{{ServerId: server, Tools: []*agentv1.AgentTool{{Name: "refund", RequireApproval: true}}}},
	}

	_, err := c.UpdateAgentDraft(ctx, connect.NewRequest(&agentv1.UpdateAgentDraftRequest{
		Id: a.PublicID, Config: &agentv1.AgentConfig{Tools: []*agentv1.AgentTool{{Name: "x"}, {Name: "x"}}},
	}))
	assert.ErrorContains(t, err, "config.tools", "duplicate tool names")
	drafted, err := c.UpdateAgentDraft(ctx, connect.NewRequest(&agentv1.UpdateAgentDraftRequest{Id: a.PublicID, Config: cfg}))
	require.NoError(t, err)
	grant := drafted.Msg.GetAgent().GetDraft().GetMcpServers()[0]
	assert.Equal(t, "refund", grant.GetTools()[0].GetName())
	assert.True(t, grant.GetTools()[0].GetRequireApproval())
	assert.True(t, drafted.Msg.GetAgent().GetUnpublishedChanges())
	assert.NotNil(t, drafted.Msg.GetAgent().GetDraftUpdatedAt())
	assert.Zero(t, drafted.Msg.GetAgent().GetPublishedVersion())

	published, err := c.PublishAgent(ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.True(t, published.Msg.GetCreated())
	assert.Equal(t, int32(1), published.Msg.GetVersion().GetVersion())
	assert.Equal(t, a.PublicID, published.Msg.GetVersion().GetAgentId())
	assert.Len(t, published.Msg.GetVersion().GetId(), 21)
	assert.Empty(t, published.Msg.GetVersion().GetCreatedBy(), "the operator is no person")
	assert.Equal(t, int32(1), published.Msg.GetAgent().GetPublishedVersion())
	assert.False(t, published.Msg.GetAgent().GetUnpublishedChanges())

	again, err := c.PublishAgent(ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.False(t, again.Msg.GetCreated())
	assert.Equal(t, published.Msg.GetVersion().GetId(), again.Msg.GetVersion().GetId())

	list, err := c.ListAgentVersions(ctx, connect.NewRequest(&agentv1.ListAgentVersionsRequest{AgentId: a.PublicID, Limit: 1}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetVersions(), 1)
	assert.Equal(t, "anthropic/claude-sonnet-5-5", list.Msg.GetVersions()[0].GetConfig().GetModel())

	for _, id := range []string{"not-a-public-id", a.ID.String()} {
		_, err := c.PublishAgent(ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: id}))
		assert.Error(t, err, id)
	}
}

func TestAgentService_PublishRecordsThePerson(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "acme")
	userID := uuid.New()
	userPub := refs.User(userID)
	refs.Resource(a.ID, a.OrgID)
	g, provider, _ := grantsFor(a, refs, "m-1", "m-2")
	a.Draft = agent.Config{ProviderID: provider, Model: "m-1"}
	svc := agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, nil).WithProviders(g).WithMCPServers(g).WithLocker(g)
	cp := agent.NewControlPlane(svc, refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceLocal, UserID: userID, ActiveOrgID: a.OrgID, Scopes: []string{"*"},
	})

	out, err := cp.PublishAgent(ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, userPub, out.Msg.GetVersion().GetCreatedBy())
	list, err := cp.ListAgentVersions(ctx, connect.NewRequest(&agentv1.ListAgentVersionsRequest{AgentId: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, userPub, list.Msg.GetVersions()[0].GetCreatedBy())

	a.Draft.Model = "m-2"
	_, err = cp.PublishAgent(ctx, connect.NewRequest(&agentv1.PublishAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	one, err := cp.ListAgentVersions(ctx, connect.NewRequest(&agentv1.ListAgentVersionsRequest{AgentId: a.PublicID, Limit: 1}))
	require.NoError(t, err)
	require.Len(t, one.Msg.GetVersions(), 1, "the request's limit reaches the store")
	assert.Equal(t, int32(2), one.Msg.GetVersions()[0].GetVersion())
}

func grantsWithServer(a *agent.Agent, refs *fakes.PublicIDs) (*fakes.AgentGrants, string) {
	g, _, _ := grantsFor(a, refs, "anthropic/claude-sonnet-5-5")
	server := uuid.New()
	g.Servers = map[uuid.UUID]fakes.GrantServer{server: {ProjectID: a.ProjectID, Name: "billing"}}
	return g, refs.MCPServer(server, a.ProjectID)
}

func TestAgentService_KnowledgeSourcesUsePublicIDs(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a, _, _ := registeredAgent(t, refs, "acme")
	store := &fakes.AgentStore{Agent: a}
	g, _ := grantsWithServer(a, refs)
	providerPub := ""
	for id := range g.Providers {
		providerPub, _, _ = refs.ProviderPublicID(context.Background(), id)
	}
	own, gone, foreign := uuid.New(), uuid.New(), uuid.New()
	g.Sources = map[uuid.UUID]uuid.UUID{own: a.ProjectID, gone: a.ProjectID, foreign: uuid.New()}
	ownPub, gonePub, foreignPub := refs.KnowledgeSource(own, a.ProjectID), refs.KnowledgeSource(gone, a.ProjectID), refs.KnowledgeSource(foreign, uuid.New())
	c := newGrantedClient(t, store, refs, g)
	ctx := context.Background()
	draft := func(ids ...string) error {
		_, err := c.UpdateAgentDraft(ctx, connect.NewRequest(&agentv1.UpdateAgentDraftRequest{Id: a.PublicID, Config: &agentv1.AgentConfig{
			ProviderId: providerPub, Model: "anthropic/claude-sonnet-5-5", KnowledgeSourceIds: ids,
		}}))
		return err
	}

	assert.ErrorContains(t, draft(foreignPub), "knowledge", "another project's source answers like an unknown one")
	require.NoError(t, draft(ownPub, gonePub))
	assert.Equal(t, []uuid.UUID{own, gone}, store.Saved.Draft.KnowledgeSourceIDs, "internal ids in the config")

	refs.Forget(gone)
	got, err := c.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, []string{ownPub}, got.Msg.GetAgent().GetDraft().GetKnowledgeSourceIds(), "public ids out; a deleted source is left out, so the next save drops it")
	assert.Equal(t, int32(1), got.Msg.GetAgent().GetDraft().GetKnowledgeSourcesMissing(), "the deleted source is counted")

	saved, err := c.UpdateAgentDraft(ctx, connect.NewRequest(&agentv1.UpdateAgentDraftRequest{Id: a.PublicID, Config: &agentv1.AgentConfig{
		ProviderId: providerPub, Model: "anthropic/claude-sonnet-5-5", KnowledgeSourceIds: []string{ownPub}, KnowledgeSourcesMissing: 7,
	}}))
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{own}, store.Saved.Draft.KnowledgeSourceIDs, "saving the draft drops the stale id")
	assert.Equal(t, int32(0), saved.Msg.GetAgent().GetDraft().GetKnowledgeSourcesMissing(), "a client-sent count is ignored")
	got, err = c.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, int32(0), got.Msg.GetAgent().GetDraft().GetKnowledgeSourcesMissing())
}
