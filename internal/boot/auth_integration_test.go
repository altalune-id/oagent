//go:build integration

package boot

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func migratedServices(t *testing.T, cfg config.Config) (*Services, config.Config) {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	cfg.Postgres.Schema = schemaName
	k := kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}, Reporter: apperror.NewReporter(nil, false)}
	svcs := buildServices(k, cfg)
	require.NoError(t, buildAuthSecrets(cfg, svcs, testLogger()))
	return svcs, cfg
}

func TestFirstBoot_EnvGenesis(t *testing.T) {
	cfg := devLocalConfig()
	cfg.Auth.Genesis = config.GenesisConfig{Email: "  Root@Example.com ", Password: "genesis-password"}
	svcs, cfg := migratedServices(t, cfg)
	ctx := t.Context()

	require.NoError(t, firstBoot(ctx, cfg, svcs, testLogger(), true))
	require.NoError(t, firstBoot(ctx, cfg, svcs, testLogger(), true), "a second boot changes nothing")

	u, err := svcs.Users.ByEmail(ctx, "root@example.com")
	require.NoError(t, err)
	assert.Equal(t, "root@example.com", u.Email)
	sys, err := svcs.Orgs.System(ctx)
	require.NoError(t, err)
	m, err := svcs.Members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	owners, err := svcs.Members.CountOwners(ctx, sys.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
	all, err := svcs.Orgs.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
	_, err = svcs.Projects.BySlug(ctx, sys.ID, project.DefaultSlug)
	require.NoError(t, err)

	p, err := svcs.Auth.LoginLocal(ctx, authCredentials("root@example.com", "genesis-password"))
	require.NoError(t, err)
	assert.True(t, p.Operator)
}

func TestFirstBoot_Basic(t *testing.T) {
	cfg := devLocalConfig()
	cfg.Auth = config.AuthConfig{Mode: config.AuthModeBasic, Basic: config.BasicConfig{User: "ops", Password: "basic-password"}}
	svcs, cfg := migratedServices(t, cfg)
	ctx := t.Context()

	require.NoError(t, firstBoot(ctx, cfg, svcs, testLogger(), true))
	sys, err := svcs.Orgs.System(ctx)
	require.NoError(t, err)
	owners, err := svcs.Members.CountOwners(ctx, sys.ID)
	require.NoError(t, err)
	assert.Zero(t, owners)
	required, err := svcs.Install.Required(ctx, true)
	require.NoError(t, err)
	assert.False(t, required)

	p, err := svcs.Auth.Basic(ctx, "ops", "basic-password")
	require.NoError(t, err)
	assert.Equal(t, sys.ID, p.ActiveOrgID)
}

func TestFirstBoot_LogsInstallURL(t *testing.T) {
	svcs, cfg := migratedServices(t, devLocalConfig())
	log, buf := bufferLogger()

	require.NoError(t, firstBoot(t.Context(), cfg, svcs, log, true))

	out := buf.String()
	assert.Contains(t, out, "level=INFO")
	assert.Contains(t, out, "http://localhost:5150/install?token=")
	assert.NotContains(t, out, " token=", "the token is only inside the url attribute")
	m := regexp.MustCompile(`/install\?token=([^"\s]+)`).FindStringSubmatch(out)
	require.Len(t, m, 2, out)
	assert.True(t, svcs.InstallToken.Matches(m[1]))
}

func TestFirstBoot_NoInstallURLWhenNotServing(t *testing.T) {
	svcs, cfg := migratedServices(t, devLocalConfig())
	log, buf := bufferLogger()

	require.NoError(t, firstBoot(t.Context(), cfg, svcs, log, false))
	assert.NotContains(t, buf.String(), "/install")
}
