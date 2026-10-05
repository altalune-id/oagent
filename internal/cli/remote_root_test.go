package cli_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/auth-server/config"
	"altalune.id/auth-server/internal/cli"
	"altalune.id/auth-server/internal/controlplane"
	"altalune.id/auth-server/internal/org"
	"altalune.id/auth-server/internal/testutil/fakes"
)

// Exercises the real root tree for a remote command: config (no validation), client boot, org.ClientFn.
func TestRoot_OrgGet_Remote(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pub", Slug: "acme", Name: "Acme"}}
	srv := controlplane.New(org.NewService(store, nil), nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	clientFn := func(context.Context, config.Config, *slog.Logger, string, string) (*controlplane.Client, error) {
		return controlplane.NewClient(ts.URL, ""), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"org", "get", store.Org.ID.String(), "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}
