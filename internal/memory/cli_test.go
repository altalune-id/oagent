package memory_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/gen/go/memory/v1/memoryv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/testutil/cptest"
)

func runMemory(t *testing.T, client memoryv1connect.MemoryServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(memory.NewCmd(func(*cobra.Command) (memoryv1connect.MemoryServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"memory"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestMemoryCmd_ListDeleteClear(t *testing.T) {
	f := newSvc(t)
	srv := controlplane.New(controlplane.Services{Memory: f.svc, Refs: f.refs}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	client := apiclient.New(ts.URL, cptest.OperatorCredential()).Memory

	buf, err := runMemory(t, client, "list", "--store", f.storeA, "--end-user", f.person, "--output", "text")
	require.NoError(t, err)
	for _, want := range []string{"ID", "AGENT_ID", "TEXT", f.agentA, f.agentB, "fact of"} {
		assert.Contains(t, buf.String(), want)
	}

	buf, err = runMemory(t, client, "list", "--store", f.storeA, "--end-user", f.person, "--agent", f.agentA, "--output", "json")
	require.NoError(t, err)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &listed))
	require.Len(t, listed, 1)
	id, _ := listed[0]["id"].(string)

	_, err = runMemory(t, client, "delete", id, "--store", f.storeA, "--end-user", f.person)
	require.NoError(t, err)
	buf, err = runMemory(t, client, "clear", "--store", f.storeA, "--end-user", f.person)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1", "clear says how many it deleted")

	_, err = runMemory(t, client, "list", "--store", f.storeA)
	assert.Error(t, err, "--end-user is required")
	_, err = runMemory(t, client, "list", "--end-user", f.person)
	assert.Error(t, err, "--store is required")
}

func TestMemoryCmd_StoreListCreateDelete(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	f.svc.WithBindings(&bindings{used: map[uuid.UUID][]memory.UsedBy{c.Scope.StoreID: {{AgentID: c.Scope.AgentID, AgentPublicID: f.agentA, Name: "Support"}}}})
	srv := controlplane.New(controlplane.Services{Memory: f.svc, Refs: f.refs}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	client := apiclient.New(ts.URL, cptest.OperatorCredential()).Memory

	buf, err := runMemory(t, client, "store", "create", "Leads", "--org-id", f.orgPub, "--project-id", f.prjPub, "--output", "text")
	require.NoError(t, err)
	id := strings.TrimSpace(buf.String())
	assert.Len(t, id, 21, "create prints the new store's public id")

	buf, err = runMemory(t, client, "store", "list", "--project-id", f.prjPub, "--output", "text")
	require.NoError(t, err)
	for _, want := range []string{"ID", "NAME", "BACKEND", "USED_BY", "Leads", "native", "Support", id} {
		assert.Contains(t, buf.String(), want)
	}
	buf, err = runMemory(t, client, "store", "list", "--project-id", f.prjPub, "--output", "json")
	require.NoError(t, err)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &listed))
	assert.Len(t, listed, 4)

	buf, err = runMemory(t, client, "store", "create", "Prospects", "--org-id", f.orgPub, "--project-id", f.prjPub,
		"--remember-api-users", "--output", "json")
	require.NoError(t, err)
	var created map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &created))
	assert.Equal(t, "Prospects", created["name"])
	assert.Equal(t, true, created["rememberApiUsers"])

	buf, err = runMemory(t, client, "store", "get", f.storeA, "--output", "text")
	require.NoError(t, err)
	for _, want := range []string{"API_USERS", "false", "Support", f.storeA} {
		assert.Contains(t, buf.String(), want)
	}
	buf, err = runMemory(t, client, "store", "get", id, "--output", "json")
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "Leads", got["name"])

	_, err = runMemory(t, client, "store", "delete", id)
	require.NoError(t, err)
	_, err = runMemory(t, client, "store", "delete", id)
	require.Error(t, err, "a deleted store is not found")
	_, err = runMemory(t, client, "store", "create", "X", "--project-id", f.prjPub)
	require.Error(t, err, "--org-id is required")
}
