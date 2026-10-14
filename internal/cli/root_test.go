package cli_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/reqid"
)

type fakeApp struct{ runErr error }

func (f fakeApp) Run(context.Context) error    { return f.runErr }
func (f fakeApp) RunMCP(context.Context) error { return f.runErr }
func (f fakeApp) Close(context.Context) error  { return nil }

func (f fakeApp) SetPassword(context.Context, string, string) error { return f.runErr }

func (f fakeApp) AddMember(context.Context, string, string, member.Role) (*member.Member, error) {
	return nil, f.runErr
}

func setServeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_HTTP_STATE_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OAGENT_SECRETS_ENCRYPTION_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	t.Setenv("OAGENT_MAIL_DRIVER", "smtp")
	t.Setenv("OAGENT_MAIL_FROM", "noreply@example.com")
	t.Setenv("OAGENT_MAIL_SMTP_HOST", "smtp.example.com")
	t.Setenv("OAGENT_HTTP_BASE_URL", "https://oagent.example.com")
}

func runRoot(t *testing.T, bootFn cli.ServerBootFn, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := cli.NewRootCmd(bootFn, nil)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	return buf, root.ExecuteContext(t.Context())
}

func TestVersion(t *testing.T) {
	buf, err := runRoot(t, nil, "version")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "oagent")
}

func TestServe_InvokesBoot(t *testing.T) {
	setServeEnv(t)
	var gotCfg config.Config
	var gotOpts int
	var gotReqID string
	bootFn := func(ctx context.Context, cfg config.Config, _ *slog.Logger, opts ...boot.Option) (cli.App, error) {
		gotCfg = cfg
		gotOpts = len(opts)
		gotReqID = reqid.FromContext(ctx)
		return fakeApp{}, nil
	}

	buf, err := runRoot(t, bootFn, "serve")

	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/db", gotCfg.Postgres.DSN)
	assert.Equal(t, 4, gotOpts)
	assert.NotEmpty(t, gotReqID)
	assert.Contains(t, buf.String(), "listening")
}

func TestServe_RunError(t *testing.T) {
	setServeEnv(t)
	bootFn := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{runErr: errors.New("boom")}, nil
	}

	_, err := runRoot(t, bootFn, "serve")
	assert.Error(t, err)
}

func TestServe_BootError(t *testing.T) {
	setServeEnv(t)
	bootFn := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return nil, errors.New("boot failed")
	}

	_, err := runRoot(t, bootFn, "serve")
	assert.Error(t, err)
}

func TestServe_ConfigError(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "")
	noop := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{}, nil
	}

	_, err := runRoot(t, noop, "serve")
	assert.Error(t, err)
}

func TestServe_InvalidLogLevel(t *testing.T) {
	setServeEnv(t)
	noop := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{}, nil
	}

	_, err := runRoot(t, noop, "serve", "--log-level", "bogus")
	assert.Error(t, err)
}

func TestServe_MutuallyExclusiveFlags(t *testing.T) {
	setServeEnv(t)
	noop := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{}, nil
	}

	_, err := runRoot(t, noop, "serve", "--scheduler-only", "--consumer-only")
	assert.Error(t, err)
}

func TestUnknownFlag(t *testing.T) {
	_, err := runRoot(t, nil, "--bogus")
	assert.Error(t, err)
}

func TestMCPCmd_InvokesRunMCP(t *testing.T) {
	setServeEnv(t)
	noop := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{}, nil
	}

	_, err := runRoot(t, noop, "mcp")
	assert.NoError(t, err)
}

func TestServe_ConfigFileMissing(t *testing.T) {
	noop := func(context.Context, config.Config, *slog.Logger, ...boot.Option) (cli.App, error) {
		return fakeApp{}, nil
	}

	_, err := runRoot(t, noop, "serve", "--config", filepath.Join(t.TempDir(), "none.yaml"))
	assert.Error(t, err)
}

func TestServe_FlagOverridesConfig(t *testing.T) {
	setServeEnv(t)
	t.Setenv("OAGENT_MODE", "cloud")
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://env/db")
	var gotCfg config.Config
	bootFn := func(_ context.Context, cfg config.Config, _ *slog.Logger, _ ...boot.Option) (cli.App, error) {
		gotCfg = cfg
		return fakeApp{}, nil
	}

	_, err := runRoot(t, bootFn, "serve", "--mode", "selfhosted", "--http-base-path", "/app", "--postgres-dsn", "postgres://flag/db")

	require.NoError(t, err)
	assert.Equal(t, config.ModeSelfhosted, gotCfg.Mode)
	assert.Equal(t, "/app", gotCfg.HTTP.BasePath)
	assert.Equal(t, "postgres://flag/db", gotCfg.Postgres.DSN)
}

func TestServe_NATSURLFlagOverridesConfig(t *testing.T) {
	setServeEnv(t)
	t.Setenv("OAGENT_NATS_URL", "nats://env:4222")
	var gotCfg config.Config
	bootFn := func(_ context.Context, cfg config.Config, _ *slog.Logger, _ ...boot.Option) (cli.App, error) {
		gotCfg = cfg
		return fakeApp{}, nil
	}

	_, err := runRoot(t, bootFn, "serve", "--queue-enabled", "--nats-url", "nats://flag:4222")

	require.NoError(t, err)
	assert.True(t, gotCfg.Queue.Enabled)
	assert.Equal(t, "nats://flag:4222", gotCfg.NATS.URL)
}
