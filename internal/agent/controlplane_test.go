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
