package channel_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
)

func runChannel(t *testing.T, client channelv1connect.ChannelServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(channel.NewCmd(func(*cobra.Command) (channelv1connect.ChannelServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"channel"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestChannelListCmd(t *testing.T) {
	refs := fakes.NewPublicIDs()
	org, project, agent := uuid.New(), uuid.New(), uuid.New()
	refs.Org(org)
	projectPub := refs.Project(project, org)
	agentPub := refs.Resource(agent, org)
	c, err := channel.New(org, project, agent, channel.Input{BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_V1StGXR8Z5jdHi6B"}, time.Now())
	require.NoError(t, err)
	c.SigningPrimarySealed, c.DeviceKeySealed = []byte("sealed-secret"), []byte("sealed-key")
	pending, err := channel.New(org, project, agent, channel.Input{BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_ZZZZZZZZZZZZZZZZ"}, time.Now())
	require.NoError(t, err)
	pending.DeviceKeySealed = []byte("sealed-key")
	store := &fakes.ChannelStore{Connections: map[uuid.UUID]*channel.Connection{c.ID: c, pending.ID: pending}}
	svc := channel.NewService(store, fakes.Transactor{}, refs, apperror.NewReporter(nil, false).Unexpected)
	srv := controlplane.New(controlplane.Services{Channels: svc, Refs: refs}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	client := apiclient.New(ts.URL, cptest.OperatorCredential()).Channels

	buf, err := runChannel(t, client, "list", "--project-id", projectPub, "--output", "text")
	require.NoError(t, err)
	out := buf.String()
	for _, want := range []string{"ID", "AGENT_ID", "ENABLED", "STATUS", "SECRET", c.PublicID, agentPub, "true", "waiting", "dev_V1StGXR8Z5jdHi6B", "set", "missing"} {
		assert.Contains(t, out, want)
	}
	for _, never := range []string{c.ID.String(), agent.String(), project.String(), "sealed"} {
		assert.NotContains(t, out, never)
	}

	buf, err = runChannel(t, client, "list", "--project-id", projectPub, "--agent-id", agentPub, "--output", "json")
	require.NoError(t, err)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rows))
	require.Len(t, rows, 2)
	byID := map[any]map[string]any{rows[0]["id"]: rows[0], rows[1]["id"]: rows[1]}
	assert.Equal(t, true, byID[c.PublicID]["secretSet"])
	assert.Equal(t, false, byID[pending.PublicID]["secretSet"])
	rows[0] = byID[c.PublicID]
	assert.Equal(t, c.PublicID, rows[0]["id"])
	assert.Equal(t, agentPub, rows[0]["agentId"])
	assert.Equal(t, true, rows[0]["enabled"])
	assert.Equal(t, "waiting", rows[0]["status"])

	_, err = runChannel(t, client, "list")
	assert.Error(t, err, "--project-id is required")
	_, err = runChannel(t, client, "list", "--project-id", uuid.NewString())
	assert.Error(t, err)
}

func TestChannelCmd_IsListOnly(t *testing.T) {
	cmd := channel.NewCmd(nil)
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Equal(t, []string{"list"}, names, "secrets never go through the CLI")
}
