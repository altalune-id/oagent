package agent_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
)

func mustAgent(t *testing.T, orgID, projectID uuid.UUID) *agent.Agent {
	t.Helper()
	a, err := agent.New(orgID, projectID, "bot-"+uuid.NewString()[:6], "Bot", "", uuid.Nil)
	require.NoError(t, err)
	return a
}

func TestAgentReach_IDOnlyRPCs(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	foreign := mustAgent(t, orgB, uuid.New())
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakes.AgentStore{Agent: foreign}
			cp := agent.NewControlPlane(agent.NewService(store, fakes.Transactor{}, nil))
			ctx := session.PrincipalInto(context.Background(), p)

			_, err := cp.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: foreign.ID.String()}))
			assert.True(t, agent.IsNotFoundError(err), "GetAgent: %v", err)
			_, err = cp.UpdateAgent(ctx, connect.NewRequest(&agentv1.UpdateAgentRequest{Id: foreign.ID.String(), Name: "Hacked"}))
			assert.True(t, agent.IsNotFoundError(err), "UpdateAgent: %v", err)
			_, err = cp.DeleteAgent(ctx, connect.NewRequest(&agentv1.DeleteAgentRequest{Id: foreign.ID.String()}))
			assert.True(t, agent.IsNotFoundError(err), "DeleteAgent: %v", err)
			assert.Nil(t, store.Saved)
			assert.Equal(t, uuid.Nil, store.Deleted)

			_, err = cp.CreateAgent(ctx, connect.NewRequest(&agentv1.CreateAgentRequest{OrgId: orgB.String(), ProjectId: foreign.ProjectID.String(), Slug: "x", Name: "X"}))
			assert.True(t, agent.IsNotFoundError(err), "CreateAgent: %v", err)
		})
	}
}

func TestAgentReach_ListFiltersToReach(t *testing.T) {
	orgA, prj := uuid.New(), uuid.New()
	mine, other := mustAgent(t, orgA, prj), mustAgent(t, orgA, prj)
	foreign := mustAgent(t, uuid.New(), prj)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agents: []*agent.Agent{mine, other, foreign}}, fakes.Transactor{}, nil))
	list := func(p session.Principal) []string {
		ctx := context.Background()
		if p.Source != "" {
			ctx = session.PrincipalInto(ctx, p)
		}
		resp, err := cp.ListAgents(ctx, connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: prj.String()}))
		require.NoError(t, err)
		var out []string
		for _, a := range resp.Msg.GetAgents() {
			out = append(out, a.GetId())
		}
		return out
	}
	assert.Empty(t, list(session.Principal{}), "no principal reaches nothing")
	assert.Len(t, list(session.System()), 3, "system sees all")
	assert.ElementsMatch(t, []string{mine.ID.String(), other.ID.String()},
		list(session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA}))
	assert.Equal(t, []string{mine.ID.String()},
		list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{mine.ID}}))
}

func TestAgentReach_OwnAgentWorks(t *testing.T) {
	orgA, prj := uuid.New(), uuid.New()
	a := mustAgent(t, orgA, prj)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, nil))
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}})
	_, err := cp.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: a.ID.String()}))
	require.NoError(t, err)
	_, err = cp.DeleteAgent(ctx, connect.NewRequest(&agentv1.DeleteAgentRequest{Id: a.ID.String()}))
	require.NoError(t, err)
}

// D7: listing a project's agents needs reach into that project; an unknown project looks the same.
func TestAgentReach_ListNeedsProjectReach(t *testing.T) {
	orgA, orgB, prjB, prjB2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	agentB := mustAgent(t, orgB, prjB)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agents: []*agent.Agent{agentB}}, fakes.Transactor{}, nil).
		WithProjects(projectOrgs{prjB: orgB, prjB2: orgB}))
	list := func(p session.Principal, project uuid.UUID) (*connect.Response[agentv1.ListAgentsResponse], error) {
		return cp.ListAgents(session.PrincipalInto(context.Background(), p), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: project.String()}))
	}

	for name, p := range map[string]session.Principal{
		"member of A":         {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":            {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
		"key of B, other prj": {Source: session.SourceAPIKey, ActiveOrgID: orgB, ProjectIDs: []uuid.UUID{prjB2}},
	} {
		_, err := list(p, prjB)
		assert.True(t, agent.IsNotFoundError(err), "%s: %v", name, err)
	}
	memberB := session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgB, Scopes: []string{"*"}}
	_, err := list(memberB, uuid.New())
	assert.True(t, agent.IsNotFoundError(err), "an unknown project: %v", err)
	resp, err := list(memberB, prjB)
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetAgents(), 1)
}
