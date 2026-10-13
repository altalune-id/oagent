package cli_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/cptest"
)

func remoteCommands() map[string][]string {
	org, project := uuid.NewString(), uuid.NewString()
	return map[string][]string{
		"org":     {"org", "list"},
		"project": {"project", "list", "--org-id", org},
		"agent":   {"agent", "list", "--project-id", project},
		"apikey":  {"apikey", "list", "--org-id", org},
		"member":  {"member", "list", "--org-id", org},
		"webhook": {"webhook", "list", "--org-id", org, "--project-id", project},
		"auth":    {"auth", "whoami"},
	}
}

func runRemote(t *testing.T, args []string, clientFn cli.ClientBootFn) error {
	t.Helper()
	t.Setenv("OAGENT_CLI_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	root := cli.NewRootCmd(nil, clientFn)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	return root.ExecuteContext(t.Context())
}

func TestRoot_EveryRemoteGroupReportsABootFailure(t *testing.T) {
	boom := errors.New("client boot failed")
	failing := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		return nil, boom
	}
	for name, args := range remoteCommands() {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, runRemote(t, args, failing), boom)
		})
	}
}

func TestRoot_EveryRemoteGroupReachesItsService(t *testing.T) {
	srv := controlplane.New(controlplane.Services{}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	booted := 0
	clientFn := func(context.Context, config.Config, *slog.Logger, string, apiclient.Credential) (*apiclient.Client, error) {
		booted++
		return apiclient.New(ts.URL, cptest.OperatorCredential()), nil
	}
	for name, args := range remoteCommands() {
		t.Run(name, func(t *testing.T) {
			before := booted
			err := runRemote(t, args, clientFn)
			require.Error(t, err, "an empty server has no services")
			assert.Equal(t, before+1, booted)
		})
	}
}
