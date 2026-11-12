package approval_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	approvalv1connect "altalune.id/oagent/gen/go/approval/v1/approvalv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/cptest"
)

func runApproval(t *testing.T, client approvalv1connect.ApprovalServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(approval.NewCmd(func(*cobra.Command) (approvalv1connect.ApprovalServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"approval"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestApprovalListCmd(t *testing.T) {
	w := newCPWorld(t)
	srv := controlplane.New(controlplane.Services{Approvals: w.svc, ApprovalRefs: w.refs, ApprovalNames: names{}}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	client := apiclient.New(ts.URL, cptest.OperatorCredential()).Approvals

	buf, err := runApproval(t, client, "list", "--project-id", w.projectPub, "--output", "text")
	require.NoError(t, err)
	out := buf.String()
	for _, want := range []string{"ID", "STATUS", "CHANNEL", "TOOLS", "REQUESTER", w.pending.PublicID, w.agentPub, "pending", "whatsapp", "record_expense", "Sari Wulandari"} {
		assert.Contains(t, out, want)
	}
	for _, never := range []string{w.pending.ID.String(), w.agent.String(), w.project.String()} {
		assert.NotContains(t, out, never, "public ids only")
	}

	buf, err = runApproval(t, client, "list", "--project-id", w.projectPub, "--agent-id", w.agentPub, "--status", "pending", "--output", "json")
	require.NoError(t, err)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, w.pending.PublicID, rows[0]["id"])
	assert.Equal(t, "Sari Wulandari", rows[0]["requesterLabel"])

	_, err = runApproval(t, client, "list")
	assert.Error(t, err, "--project-id is required")
}
