package cli_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
)

// Exercises the real root tree for a remote command: config (no validation), client boot, org.ClientFn.
func TestRoot_OrgGet_Remote(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pub", Slug: "acme", Name: "Acme"}}
	srv := controlplane.New(controlplane.Services{Orgs: org.NewService(store, nil)}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, apiclient.Credential{}), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"org", "get", store.Org.ID.String(), "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}

func TestRoot_ProjectGet_Remote(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	srv := controlplane.New(controlplane.Services{Projects: project.NewService(&fakes.ProjectStore{Project: p}, nil)}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, apiclient.Credential{}), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"project", "get", p.ID.String(), "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}

func TestRoot_AgentGet_Remote(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	srv := controlplane.New(controlplane.Services{Agents: agent.NewService(&fakes.AgentStore{Agent: a}, nil)}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, apiclient.Credential{}), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"agent", "get", a.ID.String(), "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}
