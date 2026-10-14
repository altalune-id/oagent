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
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
)

// Exercises the real root tree for a remote command: config (no validation), client boot, org.ClientFn.
func TestRoot_OrgGet_Remote(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pubpubpubpubpubpubpub", Slug: "acme", Name: "Acme"}}
	srv := controlplane.New(controlplane.Services{Orgs: org.NewService(store, fakes.Transactor{}, nil)}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, cptest.OperatorCredential()), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"org", "get", store.Org.PublicID, "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
	assert.Contains(t, buf.String(), `"id": "pubpubpubpubpubpubpub"`)
	assert.NotContains(t, buf.String(), store.Org.ID.String())
}

func TestRoot_ProjectGet_Remote(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	orgPublicID := refs.Org(p.OrgID)
	srv := controlplane.New(controlplane.Services{
		Projects: project.NewService(&fakes.ProjectStore{Project: p}, nil),
		Refs:     refs,
	}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, cptest.OperatorCredential()), nil
	}
	run := func(id string) (string, error) {
		buf := new(bytes.Buffer)
		root := cli.NewRootCmd(nil, clientFn)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"project", "get", id, "--output", "json"})
		err := root.ExecuteContext(t.Context())
		return buf.String(), err
	}

	out, err := run(p.PublicID)
	require.NoError(t, err)
	assert.Contains(t, out, `"slug": "acme"`)
	assert.Contains(t, out, `"id": "`+p.PublicID+`"`)
	assert.Contains(t, out, orgPublicID)
	assert.NotContains(t, out, p.ID.String())

	_, err = run(p.ID.String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_argument", "a UUID is not a public id")

	_, err = run("unknwnunknwnunknwnabc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_found", "a well-formed unknown id")
}

func TestRoot_AgentGet_Remote(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	refs.Org(a.OrgID)
	refs.Project(a.ProjectID, a.OrgID)
	srv := controlplane.New(controlplane.Services{
		Agents: agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, nil),
		Refs:   refs,
	}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)

	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return apiclient.New(ts.URL, cptest.OperatorCredential()), nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(nil, clientFn)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"agent", "get", a.PublicID, "--output", "json"})

	require.NoError(t, root.ExecuteContext(t.Context()))
	assert.Contains(t, buf.String(), `"slug": "acme"`)
	assert.Contains(t, buf.String(), `"id": "`+a.PublicID+`"`)
	assert.NotContains(t, buf.String(), a.ID.String())
}
