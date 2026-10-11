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

func newAgentServiceClient(t *testing.T, store agent.Store) agentv1connect.AgentServiceClient {
	t.Helper()
	_, handler := agentv1connect.NewAgentServiceHandler(agent.NewControlPlane(agent.NewService(store, nil)))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return agentv1connect.NewAgentServiceClient(http.DefaultClient, ts.URL)
}

func TestAgentService_CreateAgent(t *testing.T) {
	c := newAgentServiceClient(t, &fakes.AgentStore{})

	resp, err := c.CreateAgent(context.Background(), connect.NewRequest(&agentv1.CreateAgentRequest{
		OrgId: uuid.New().String(), ProjectId: uuid.New().String(), Slug: "acme", Name: "Acme", Description: "d",
	}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetAgent().GetSlug())
	assert.NotEmpty(t, resp.Msg.GetAgent().GetProjectId())
}

func TestAgentService_CreateAgent_InvalidScope(t *testing.T) {
	c := newAgentServiceClient(t, &fakes.AgentStore{})

	_, err := c.CreateAgent(context.Background(), connect.NewRequest(&agentv1.CreateAgentRequest{ProjectId: uuid.New().String(), Name: "Acme"}))
	assert.Error(t, err)

	_, err = c.CreateAgent(context.Background(), connect.NewRequest(&agentv1.CreateAgentRequest{OrgId: uuid.New().String(), Name: "Acme"}))
	assert.Error(t, err)
}

func TestAgentService_GetAgent(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a})

	resp, err := c.GetAgent(context.Background(), connect.NewRequest(&agentv1.GetAgentRequest{Id: a.ID.String()}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetAgent().GetSlug())
}

func TestAgentService_GetAgent_InvalidID(t *testing.T) {
	c := newAgentServiceClient(t, &fakes.AgentStore{})

	_, err := c.GetAgent(context.Background(), connect.NewRequest(&agentv1.GetAgentRequest{Id: "not-a-uuid"}))
	assert.Error(t, err)
}

func TestAgentService_ListAgents(t *testing.T) {
	a1, err := agent.New(uuid.New(), uuid.New(), "aaa", "A", "", uuid.New())
	require.NoError(t, err)
	a2, err := agent.New(uuid.New(), uuid.New(), "bbb", "B", "", uuid.New())
	require.NoError(t, err)
	c := newAgentServiceClient(t, &fakes.AgentStore{Agents: []*agent.Agent{a1, a2}})

	resp, err := c.ListAgents(context.Background(), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: uuid.New().String()}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetAgents(), 2)
}

func TestAgentService_ListAgents_InvalidProject(t *testing.T) {
	c := newAgentServiceClient(t, &fakes.AgentStore{})

	_, err := c.ListAgents(context.Background(), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: "bad"}))
	assert.Error(t, err)
}

func TestAgentService_UpdateAgent(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a})

	resp, err := c.UpdateAgent(context.Background(), connect.NewRequest(&agentv1.UpdateAgentRequest{Id: a.ID.String(), Name: "Acme Two", Description: "x"}))
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", resp.Msg.GetAgent().GetName())
}

func TestAgentService_DeleteAgent(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	c := newAgentServiceClient(t, &fakes.AgentStore{Agent: a})

	_, err = c.DeleteAgent(context.Background(), connect.NewRequest(&agentv1.DeleteAgentRequest{Id: a.ID.String()}))
	assert.NoError(t, err)

	_, err = c.DeleteAgent(context.Background(), connect.NewRequest(&agentv1.DeleteAgentRequest{Id: "bad"}))
	assert.Error(t, err)
}
