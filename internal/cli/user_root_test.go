package cli_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/user"
)

type passwordApp struct {
	fakeApp
	email, password string
	err             error
	closed          bool
}

func (a *passwordApp) SetPassword(_ context.Context, email, password string) error {
	a.email, a.password = email, password
	return a.err
}

func (a *passwordApp) Close(context.Context) error {
	a.closed = true
	return nil
}

func runSetPassword(t *testing.T, app *passwordApp, stdin string, args ...string) (string, int, error) {
	t.Helper()
	setServeEnv(t)
	var opts int
	bootFn := func(_ context.Context, _ config.Config, _ *slog.Logger, o ...boot.Option) (cli.App, error) {
		opts = len(o)
		return app, nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(bootFn, nil)
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"user", "set-password"}, args...))
	err := root.ExecuteContext(t.Context())
	return buf.String(), opts, err
}

func TestUserSetPassword_BootsWithoutWorkers(t *testing.T) {
	app := &passwordApp{}
	_, opts, err := runSetPassword(t, app, "a brand new password\n", "--email", "ada@example.com", "--password-stdin")
	require.NoError(t, err)

	assert.Equal(t, "ada@example.com", app.email)
	assert.Equal(t, "a brand new password", app.password)
	assert.True(t, app.closed)
	assert.Equal(t, 3, opts, "install URL, scheduler and consumer are off")
}

func TestUserSetPassword_AppError(t *testing.T) {
	app := &passwordApp{err: &user.NotFoundError{}}
	_, _, err := runSetPassword(t, app, "a brand new password\n", "--email", "nobody@example.com", "--password-stdin")
	require.Error(t, err)
	assert.True(t, user.IsNotFoundError(err))
	assert.True(t, app.closed)
}

// SECURITY: set-password changes the database in the local config, so a remote flag must never be silently ignored.
func TestUserSetPassword_RefusesRemoteFlags(t *testing.T) {
	for _, flag := range []string{"url", "token", "token-file", "profile"} {
		t.Run(flag, func(t *testing.T) {
			app := &passwordApp{}
			_, opts, err := runSetPassword(t, app, "a brand new password\n", "--email", "ada@example.com", "--password-stdin", "--"+flag, "x")

			require.Error(t, err)
			assert.Zero(t, opts, "the server never boots")
			assert.Empty(t, app.email)
			assert.Equal(t, cli.ExitInvalidArg, cli.ExitCodeFor(err))
			assert.Contains(t, err.Error(), "user set-password runs on the server with database access")
			assert.Contains(t, err.Error(), "--"+flag)
		})
	}
}
