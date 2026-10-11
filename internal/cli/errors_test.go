package cli

import (
	"bytes"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
)

func renderRoot(t *testing.T, format string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--output", format})
	require.NoError(t, root.Execute())
	return root, buf
}

func connectNotFound() *connect.Error {
	cerr := connect.NewError(connect.CodeNotFound, errors.New("Organization not found"))
	det, _ := connect.NewErrorDetail(&apperrorv1.ErrorDetail{Code: "ORG001"})
	cerr.AddDetail(det)
	return cerr
}

func TestRenderError_JSON(t *testing.T) {
	root, buf := renderRoot(t, "json")
	RenderError(root, connectNotFound())
	assert.Contains(t, buf.String(), `"error": "Organization not found"`)
	assert.Contains(t, buf.String(), `"code": "ORG001"`)
}

func TestRenderError_NDJSON(t *testing.T) {
	root, buf := renderRoot(t, "ndjson")
	RenderError(root, connectNotFound())
	assert.Contains(t, buf.String(), `"code":"ORG001"`)
}

func TestRenderError_Text(t *testing.T) {
	root, buf := renderRoot(t, "text")
	RenderError(root, apperror.New(apperror.CodeOrgNotFound, "Organization not found"))
	assert.Contains(t, buf.String(), "error: Organization not found (ORG001)")
}

func TestRenderError_Plain(t *testing.T) {
	root, buf := renderRoot(t, "text")
	RenderError(root, errors.New("boom"))
	assert.Contains(t, buf.String(), "error: boom")
}

func TestRenderError_UnauthenticatedHint(t *testing.T) {
	cerr := connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	det, _ := connect.NewErrorDetail(&apperrorv1.ErrorDetail{Code: string(apperror.CodeUnauthenticated)})
	cerr.AddDetail(det)

	root, buf := renderRoot(t, "text")
	RenderError(root, cerr)
	assert.Contains(t, buf.String(), "not signed in or credential rejected; run `oagent auth login` or pass --token (GEN002)")
	assert.Equal(t, ExitUnauth, ExitCodeFor(cerr))
}
