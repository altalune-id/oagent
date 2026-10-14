//go:build integration

package boot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

func TestAddMember(t *testing.T) {
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	sqlDB, schemaName := pgtest.Migrated(t)
	cfg.Postgres.Schema = schemaName
	svcs := buildServices(kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}, Reporter: apperror.NewReporter(nil, false)}, cfg)
	require.NoError(t, buildAuthSecrets(cfg, svcs, testLogger()))
	svcs.Caps.EnvGenesis = true
	ctx := t.Context()
	require.NoError(t, firstBoot(ctx, cfg, svcs, testLogger(), false))
	sys, err := svcs.Orgs.System(ctx)
	require.NoError(t, err)
	bob, err := svcs.Users.Create(ctx, user.CreateRequest{Email: "bob@example.com", Name: "Bob", Password: "a long enough password"})
	require.NoError(t, err)
	app := &App{Services: svcs}

	m, err := app.AddMember(ctx, "  Bob@Example.com ", sys.PublicID, member.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, sys.ID, m.OrgID)
	assert.Equal(t, bob.ID, m.UserID)
	assert.Equal(t, bob.PublicID, m.UserPublicID)
	got, err := svcs.Members.Find(ctx, sys.ID, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, got.Role)
	var queued int
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM mail_queue`).Scan(&queued))
	assert.Zero(t, queued, "adding an existing user queues no email")

	_, err = app.AddMember(ctx, "bob@example.com", sys.PublicID, member.RoleMember)
	assert.True(t, member.IsAlreadyMemberError(err), "an existing membership is never changed")

	_, err = app.AddMember(ctx, "nobody@example.com", sys.PublicID, member.RoleMember)
	require.True(t, user.IsUnknownEmailError(err))
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, "No user with email nobody@example.com", ae.Message())

	for _, notPublic := range []string{sys.Slug, sys.ID.String()} {
		_, err = app.AddMember(ctx, "bob@example.com", notPublic, member.RoleMember)
		assert.True(t, org.IsNotFoundError(err), "%q is not an org's public id", notPublic)
	}
}
