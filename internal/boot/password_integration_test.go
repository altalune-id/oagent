//go:build integration

package boot

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/auth"
)

// Review Focus 5: a password reset ends every session of that user.
func TestSetPassword_EndsSessions(t *testing.T) {
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	ctx := t.Context()

	p, err := svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "genesis-password"})
	require.NoError(t, err)
	require.NoError(t, svcs.Sessions.Save(ctx, "old-sid", p, time.Now().Add(time.Hour)))

	app := &App{Services: svcs}
	require.NoError(t, app.SetPassword(ctx, "Root@Example.com", "a brand new password"))

	_, ok, err := svcs.Sessions.Load(ctx, "old-sid")
	require.NoError(t, err)
	assert.False(t, ok, "the old session is gone")

	_, err = svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "a brand new password"})
	require.NoError(t, err, "the new password signs in")
	_, err = svcs.Auth.LoginLocal(ctx, auth.Credentials{Email: "root@example.com", Password: "genesis-password"})
	assert.True(t, auth.IsInvalidCredentialsError(err), "the old password no longer signs in")
}
