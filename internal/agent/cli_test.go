package agent_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
)

func agentClient(t *testing.T, store agent.Store, refs *fakes.PublicIDs) agentv1connect.AgentServiceClient {
	t.Helper()
	srv := controlplane.New(controlplane.Services{Agents: agent.NewService(store, fakes.Transactor{}, nil), Refs: refs}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return apiclient.New(ts.URL, cptest.OperatorCredential()).Agents
}

func cliAgent(t *testing.T, refs *fakes.PublicIDs, slug string) *agent.Agent {
	t.Helper()
	orgID, projectID := uuid.New(), uuid.New()
	refs.Org(orgID)
	refs.Project(projectID, orgID)
	a, err := agent.New(orgID, projectID, slug, "Acme", "", uuid.New())
	require.NoError(t, err)
	return a
}

func runAgent(t *testing.T, client agentv1connect.AgentServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(agent.NewCmd(func(*cobra.Command) (agentv1connect.AgentServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"agent"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestAgentCreate(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID, projectID := uuid.New(), uuid.New()
	store := &fakes.AgentStore{}
	buf, err := runAgent(t, agentClient(t, store, refs), "create",
		"--org-id", refs.Org(orgID), "--project-id", refs.Project(projectID, orgID), "--name", "Acme", "--slug", "acme", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created agent acme ("+store.Saved.PublicID+")")
}

func TestAgentGet_Text(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a := cliAgent(t, refs, "acme")

	buf, err := runAgent(t, agentClient(t, &fakes.AgentStore{Agent: a}, refs), "get", a.PublicID, "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "acme")
	assert.Contains(t, buf.String(), a.PublicID)
	assert.NotContains(t, buf.String(), a.ID.String())
	assert.NotContains(t, buf.String(), a.ProjectID.String())
}

func TestAgentGet_UnknownID(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a := cliAgent(t, refs, "acme")

	_, err := runAgent(t, agentClient(t, &fakes.AgentStore{Agent: a}, refs), "get", a.ID.String())
	assert.ErrorContains(t, err, "invalid_argument", "a UUID is not a public id")

	_, err = runAgent(t, agentClient(t, &fakes.AgentStore{Agent: a}, refs), "get", "unknwnunknwnunknwnabc")
	assert.ErrorContains(t, err, "not_found")
}

func TestAgentList_JSON(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a := cliAgent(t, refs, "acme")
	projectPub := refs.Project(a.ProjectID, a.OrgID)

	buf, err := runAgent(t, agentClient(t, &fakes.AgentStore{Agents: []*agent.Agent{a}}, refs), "list", "--project-id", projectPub, "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
	assert.Contains(t, buf.String(), `"id": "`+a.PublicID+`"`)
}

func TestAgentUpdate(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a := cliAgent(t, refs, "acme")

	buf, err := runAgent(t, agentClient(t, &fakes.AgentStore{Agent: a}, refs), "update", a.PublicID, "--name", "Acme Two", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Updated agent acme ("+a.PublicID+")")
}

func TestAgentDelete(t *testing.T) {
	refs := fakes.NewPublicIDs()
	a := cliAgent(t, refs, "bot")
	store := &fakes.AgentStore{Agent: a}
	buf, err := runAgent(t, agentClient(t, store, refs), "delete", a.PublicID)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Deleted agent")
	assert.Equal(t, a.ID, store.Deleted)
}
