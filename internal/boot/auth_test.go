package boot

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
)

const testKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func authKernel(t *testing.T, log *slog.Logger) kernel.Kernel {
	t.Helper()
	sqlDB, _ := newDB(t)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return kernel.Kernel{Log: log, Pool: db.Pool{W: sqlDB, R: sqlDB}, Reporter: apperror.NewReporter(nil, false)}
}

func devLocalConfig() config.Config {
	return config.Config{
		Mode:     config.ModeSelfhosted,
		Env:      config.EnvDevelopment,
		HTTP:     config.HTTPConfig{Addr: ":5150"},
		Postgres: config.PostgresConfig{Schema: "public"},
		Auth:     config.AuthConfig{Mode: config.AuthModeLocal},
	}
}

func TestBuildServices_AuthDevelopmentWithoutSecrets(t *testing.T) {
	log, buf := bufferLogger()
	k := authKernel(t, log)
	cfg := devLocalConfig()
	svcs := buildServices(k, cfg)

	require.NoError(t, buildAuthSecrets(cfg, svcs, log))

	assert.IsType(t, &session.MemoryStore{}, svcs.Sessions)
	assert.Len(t, svcs.StateSecret, 32)
	assert.NotNil(t, svcs.Auth)
	assert.NotNil(t, svcs.Install)
	assert.NotNil(t, svcs.Users)
	assert.NotNil(t, svcs.Members)
	assert.True(t, svcs.Caps.LocalLogin)
	assert.Len(t, svcs.InstallToken.Value(), 32)
	assert.Equal(t, 2, strings.Count(buf.String(), "level=WARN"), buf.String())
	assert.Contains(t, buf.String(), "http.stateSecret is missing or invalid")
	assert.Contains(t, buf.String(), "secrets.encryptionKey is missing or invalid")

	other := buildServices(k, cfg)
	require.NoError(t, buildAuthSecrets(cfg, other, log))
	assert.NotEqual(t, svcs.StateSecret, other.StateSecret, "a generated state secret is random per boot")
}

func TestBuildServices_AuthWithSecrets(t *testing.T) {
	log, buf := bufferLogger()
	k := authKernel(t, log)
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Install.Token = strings.Repeat("t", 32)
	svcs := buildServices(k, cfg)

	require.NoError(t, buildAuthSecrets(cfg, svcs, log))

	assert.IsType(t, &session.PostgresStore{}, svcs.Sessions)
	assert.Equal(t, []byte(cfg.HTTP.StateSecret), svcs.StateSecret)
	assert.Equal(t, cfg.Install.Token, svcs.InstallToken.Value())
	assert.NotContains(t, buf.String(), "level=WARN")
}

func TestBuildServices_AuthNoSecretWarningsWithoutLocalLogin(t *testing.T) {
	log, buf := bufferLogger()
	k := authKernel(t, log)
	cfg := devLocalConfig()
	cfg.Auth = config.AuthConfig{Mode: config.AuthModeBasic, Basic: config.BasicConfig{User: "ops", Password: "s3cret-pass"}}
	svcs := buildServices(k, cfg)

	require.NoError(t, buildAuthSecrets(cfg, svcs, log))

	assert.False(t, svcs.Caps.LocalLogin)
	assert.IsType(t, &session.MemoryStore{}, svcs.Sessions)
	assert.Len(t, svcs.StateSecret, 32)
	assert.NotContains(t, buf.String(), "level=WARN")
}

func TestFirstBoot_NoopWithoutLocalOrBasic(t *testing.T) {
	log, buf := bufferLogger()
	k := authKernel(t, log)
	cfg := config.Config{}
	svcs := buildServices(k, cfg)
	require.NoError(t, buildAuthSecrets(cfg, svcs, log))

	require.NoError(t, firstBoot(context.Background(), cfg, svcs, log, true))
	assert.NotContains(t, buf.String(), "/install")
}

func TestInstallURL(t *testing.T) {
	for name, tc := range map[string]struct {
		http config.HTTPConfig
		want string
	}{
		"default port":      {config.HTTPConfig{Addr: ":5150"}, "http://localhost:5150/install?token=tok"},
		"host and port":     {config.HTTPConfig{Addr: "0.0.0.0:8080", BasePath: "/app"}, "http://localhost:8080/app/install?token=tok"},
		"base url":          {config.HTTPConfig{Addr: ":5150", BaseURL: "https://oagent.example.com/"}, "https://oagent.example.com/install?token=tok"},
		"base url and path": {config.HTTPConfig{Addr: ":5150", BaseURL: "https://example.com", BasePath: "/oagent"}, "https://example.com/oagent/install?token=tok"},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, installURL(tc.http, "tok")) })
	}
}

func TestPublicBase(t *testing.T) {
	for name, tc := range map[string]struct {
		http config.HTTPConfig
		want string
	}{
		"default port":      {config.HTTPConfig{Addr: ":5150"}, "http://localhost:5150"},
		"no port":           {config.HTTPConfig{}, "http://localhost"},
		"base path":         {config.HTTPConfig{Addr: "0.0.0.0:8080", BasePath: "/app/"}, "http://localhost:8080/app"},
		"base url and path": {config.HTTPConfig{Addr: ":5150", BaseURL: "https://example.com/", BasePath: "/oagent"}, "https://example.com/oagent"},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, publicBase(tc.http)) })
	}
}

func authCredentials(email, pass string) auth.Credentials {
	return auth.Credentials{Email: email, Password: pass}
}

func TestLogsInstallURL(t *testing.T) {
	assert.True(t, logsInstallURL(newOptions()))
	for name, opt := range map[string]Option{
		"mcp stdio":      WithInstallURL(false),
		"scheduler only": WithSchedulerOnly(true),
		"consumer only":  WithConsumerOnly(true),
	} {
		o := newOptions()
		opt(o)
		assert.False(t, logsInstallURL(o), name)
	}
}

func TestBuildAuthSecrets_WarnsOnInsecureCookieOutsideDevelopment(t *testing.T) {
	log, buf := bufferLogger()
	k := authKernel(t, log)
	cfg := devLocalConfig()
	cfg.Env = config.EnvProduction
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	svcs := buildServices(k, cfg)

	require.NoError(t, buildAuthSecrets(cfg, svcs, log))
	assert.Contains(t, buf.String(), "boot: http.cookieSecure is false; session cookies are sent over plain HTTP")

	buf.Reset()
	cfg.HTTP.CookieSecure = true
	require.NoError(t, buildAuthSecrets(cfg, buildServices(k, cfg), log))
	assert.NotContains(t, buf.String(), "cookieSecure")
}

func TestBuildServices_OrgCreationRefusedInSelfhosted(t *testing.T) {
	svcs := buildServices(authKernel(t, nil), devLocalConfig())
	_, err := svcs.Orgs.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	require.Error(t, err)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeOrgCreationDisabled, ae.Code())
}
