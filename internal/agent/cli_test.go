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
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/fakes"
)

func agentClient(t *testing.T, store agent.Store) agentv1connect.AgentServiceClient {
	t.Helper()
	srv := controlplane.New(controlplane.Services{Agents: agent.NewService(store, nil)}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return controlplane.NewClient(ts.URL, "").Agents
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
	buf, err := runAgent(t, agentClient(t, &fakes.AgentStore{}), "create",
		"--org-id", uuid.New().String(), "--project-id", uuid.New().String(), "--name", "Acme", "--slug", "acme", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created agent acme")
}

func TestAgentGet_Text(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	store := &fakes.AgentStore{Agent: a}

	buf, err := runAgent(t, agentClient(t, store), "get", a.ID.String(), "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "acme")
}

func TestAgentList_JSON(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	store := &fakes.AgentStore{Agents: []*agent.Agent{a}}

	buf, err := runAgent(t, agentClient(t, store), "list", "--project-id", a.ProjectID.String(), "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}

func TestAgentUpdate(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	store := &fakes.AgentStore{Agent: a}

	buf, err := runAgent(t, agentClient(t, store), "update", a.ID.String(), "--name", "Acme Two", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Updated agent acme")
}

func TestAgentDelete(t *testing.T) {
	buf, err := runAgent(t, agentClient(t, &fakes.AgentStore{}), "delete", uuid.New().String())

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Deleted agent")
}
