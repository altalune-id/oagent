package cli_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
)

type memberAddApp struct {
	fakeApp
	email, org string
	role       member.Role
	err        error
	closed     bool
}

func (a *memberAddApp) AddMember(_ context.Context, email, orgPublicID string, role member.Role) (*member.Member, error) {
	a.email, a.org, a.role = email, orgPublicID, role
	if a.err != nil {
		return nil, a.err
	}
	return &member.Member{Membership: member.Membership{Role: role}}, nil
}

func (a *memberAddApp) Close(context.Context) error {
	a.closed = true
	return nil
}

func runMemberAdd(t *testing.T, app *memberAddApp, args ...string) (string, int, bool, error) {
	t.Helper()
	opts, booted := 0, false
	bootFn := func(_ context.Context, _ config.Config, _ *slog.Logger, o ...boot.Option) (cli.App, error) {
		opts, booted = len(o), true
		return app, nil
	}
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(bootFn, nil)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"member", "add"}, args...))
	err := root.ExecuteContext(t.Context())
	if err != nil {
		cli.RenderError(root, err)
	}
	return buf.String(), opts, booted, err
}

func TestMemberAdd_BootsWithoutWorkers(t *testing.T) {
	setServeEnv(t)
	app := &memberAddApp{}
	out, opts, _, err := runMemberAdd(t, app, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a", "--role", "admin", "--output", "text")
	require.NoError(t, err)

	assert.Equal(t, "ada@example.com", app.email)
	assert.Equal(t, "V1StGXR8Z5jdHi6BmyT2a", app.org)
	assert.Equal(t, member.RoleAdmin, app.role)
	assert.True(t, app.closed)
	assert.Equal(t, 3, opts, "install URL, scheduler and consumer are off")
	assert.Contains(t, out, "Added ada@example.com to V1StGXR8Z5jdHi6BmyT2a as admin")
}

func TestMemberAdd_ValidatesServerConfig(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "")
	_, _, booted, err := runMemberAdd(t, &memberAddApp{}, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a")

	require.Error(t, err, "member add is an operator command: it needs the server config, not --url")
	assert.False(t, booted)
}

func TestMemberAdd_UnknownEmail(t *testing.T) {
	setServeEnv(t)
	app := &memberAddApp{err: &user.UnknownEmailError{Email: "nobody@example.com"}}
	out, _, _, err := runMemberAdd(t, app, "--email", "nobody@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a")

	require.Error(t, err)
	assert.Contains(t, out, "No user with email nobody@example.com")
	assert.Equal(t, cli.ExitNotFound, cli.ExitCodeFor(err))
	assert.True(t, app.closed)
}

func TestMemberAdd_AlreadyMember(t *testing.T) {
	setServeEnv(t)
	out, _, _, err := runMemberAdd(t, &memberAddApp{err: &member.AlreadyMemberError{}}, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a")

	assert.True(t, member.IsAlreadyMemberError(err))
	assert.Contains(t, out, "User is already a member")
	assert.Equal(t, cli.ExitAlreadyExists, cli.ExitCodeFor(err))
}

// SECURITY: member add changes the database in the local config, so a remote flag must never be silently ignored.
func TestMemberAdd_RefusesRemoteFlags(t *testing.T) {
	for _, flag := range []string{"url", "token", "token-file", "profile"} {
		t.Run(flag, func(t *testing.T) {
			setServeEnv(t)
			app := &memberAddApp{}
			out, _, booted, err := runMemberAdd(t, app, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a", "--"+flag, "x")

			require.Error(t, err)
			assert.False(t, booted, "the local database is never touched")
			assert.Equal(t, cli.ExitInvalidArg, cli.ExitCodeFor(err))
			assert.Contains(t, out, "member add runs on the server with database access")
			assert.Contains(t, out, "--"+flag)
		})
	}
}
