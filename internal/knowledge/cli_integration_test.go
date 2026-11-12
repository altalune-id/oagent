//go:build integration

package knowledge_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/gen/go/knowledge/v1/knowledgev1connect"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
)

func runKnowledge(t *testing.T, client knowledgev1connect.KnowledgeServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(knowledge.NewCmd(func(*cobra.Command) (knowledgev1connect.KnowledgeServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"knowledge"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestKnowledgeListCmd(t *testing.T) {
	e := newEnv(t)
	c := newCP(t, e, e.svc)
	src := e.uploadSource(t, "Policies")
	e.urlSource(t, e.svc)
	_, handler := knowledgev1connect.NewKnowledgeServiceHandler(c.client, connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	client := knowledgev1connect.NewKnowledgeServiceClient(http.DefaultClient, ts.URL)

	buf, err := runKnowledge(t, client, "list", "--project-id", c.prjPub, "--output", "text")
	require.NoError(t, err)
	for _, want := range []string{"ID", "NAME", "KIND", "STATUS", "READY", src.PublicID, "Policies", "upload", "idle", "Help", "url", "crawling", "https://help.example.com"} {
		assert.Contains(t, buf.String(), want)
	}
	assert.NotContains(t, buf.String(), src.ID.String(), "public ids only")

	buf, err = runKnowledge(t, client, "list", "--project-id", c.prjPub, "--output", "json")
	require.NoError(t, err)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rows))
	require.Len(t, rows, 2)
	assert.Equal(t, src.PublicID, rows[0]["id"])
	assert.Equal(t, "pgvector", rows[0]["backend"])

	_, err = runKnowledge(t, client, "list")
	assert.Error(t, err, "--project-id is required")
}
