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
	refs := fakes.NewPublicIDs()
	orgA, orgB, prjB := uuid.New(), uuid.New(), uuid.New()
	refs.Org(orgA)
	orgBPub, prjBPub := refs.Org(orgB), refs.Project(prjB, orgB)
	foreign := mustAgent(t, orgB, prjB)
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakes.AgentStore{Agent: foreign}
			cp := agent.NewControlPlane(agent.NewService(store, fakes.Transactor{}, nil), refs)
			ctx := session.PrincipalInto(context.Background(), p)

			for _, id := range []string{foreign.PublicID, foreign.ID.String()} {
				_, err := cp.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: id}))
				assert.True(t, agent.IsNotFoundError(err), "GetAgent %s: %v", id, err)
				_, err = cp.UpdateAgent(ctx, connect.NewRequest(&agentv1.UpdateAgentRequest{Id: id, Name: "Hacked"}))
				assert.True(t, agent.IsNotFoundError(err), "UpdateAgent %s: %v", id, err)
				_, err = cp.DeleteAgent(ctx, connect.NewRequest(&agentv1.DeleteAgentRequest{Id: id}))
				assert.True(t, agent.IsNotFoundError(err), "DeleteAgent %s: %v", id, err)
			}
			assert.Nil(t, store.Saved)
			assert.Equal(t, uuid.Nil, store.Deleted)

			for _, req := range []*agentv1.CreateAgentRequest{
				{OrgId: orgBPub, ProjectId: prjBPub, Slug: "x", Name: "X"},
				{OrgId: orgB.String(), ProjectId: prjB.String(), Slug: "x", Name: "X"},
			} {
				_, err := cp.CreateAgent(ctx, connect.NewRequest(req))
				assert.True(t, agent.IsNotFoundError(err), "CreateAgent %s: %v", req.GetProjectId(), err)
			}
			assert.Nil(t, store.Saved)
		})
	}
}

func TestAgentReach_ListFiltersToReach(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	prjPub := refs.Project(prj, orgA)
	mine, other := mustAgent(t, orgA, prj), mustAgent(t, orgA, prj)
	foreign := mustAgent(t, uuid.New(), prj)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agents: []*agent.Agent{mine, other, foreign}}, fakes.Transactor{}, nil), refs)
	list := func(p session.Principal) ([]string, error) {
		ctx := context.Background()
		if p.Source != "" {
			ctx = session.PrincipalInto(ctx, p)
		}
		resp, err := cp.ListAgents(ctx, connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: prjPub}))
		if err != nil {
			return nil, err
		}
		var out []string
		for _, a := range resp.Msg.GetAgents() {
			out = append(out, a.GetId())
		}
		return out, nil
	}
	_, err := list(session.Principal{})
	assert.True(t, agent.IsNotFoundError(err), "no principal reaches nothing: %v", err)
	got, err := list(session.System())
	require.NoError(t, err)
	assert.Len(t, got, 3, "system sees all")
	got, err = list(session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mine.PublicID, other.PublicID}, got)
	got, err = list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{mine.ID}})
	require.NoError(t, err)
	assert.Equal(t, []string{mine.PublicID}, got)
}

func TestAgentReach_OwnAgentWorks(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	refs.Project(prj, orgA)
	a := mustAgent(t, orgA, prj)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, nil), refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}})
	resp, err := cp.GetAgent(ctx, connect.NewRequest(&agentv1.GetAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, a.PublicID, resp.Msg.GetAgent().GetId())
	_, err = cp.DeleteAgent(ctx, connect.NewRequest(&agentv1.DeleteAgentRequest{Id: a.PublicID}))
	require.NoError(t, err)
}

// D7: listing a project's agents needs reach into that project; an unknown project looks the same.
func TestAgentReach_ListNeedsProjectReach(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, orgB, prjB, prjB2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	refs.Org(orgA)
	refs.Org(orgB)
	prjBPub := refs.Project(prjB, orgB)
	refs.Project(prjB2, orgB)
	agentB := mustAgent(t, orgB, prjB)
	cp := agent.NewControlPlane(agent.NewService(&fakes.AgentStore{Agents: []*agent.Agent{agentB}}, fakes.Transactor{}, nil), refs)
	list := func(p session.Principal, project string) (*connect.Response[agentv1.ListAgentsResponse], error) {
		return cp.ListAgents(session.PrincipalInto(context.Background(), p), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: project}))
	}

	for name, p := range map[string]session.Principal{
		"member of A":         {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":            {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
		"key of B, other prj": {Source: session.SourceAPIKey, ActiveOrgID: orgB, ProjectIDs: []uuid.UUID{prjB2}},
	} {
		_, err := list(p, prjBPub)
		assert.True(t, agent.IsNotFoundError(err), "%s: %v", name, err)
	}
	memberB := session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgB, Scopes: []string{"*"}}
	for _, id := range []string{"unknown-project-id", prjB.String()} {
		_, err := list(memberB, id)
		assert.True(t, agent.IsNotFoundError(err), "an unknown project %s: %v", id, err)
	}
	resp, err := list(memberB, prjBPub)
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetAgents(), 1)
	assert.Equal(t, agentB.PublicID, resp.Msg.GetAgents()[0].GetId())
}
