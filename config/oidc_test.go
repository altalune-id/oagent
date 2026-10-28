package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oidcOn(c *Config) {
	c.OIDC.Issuer = "https://idp.example.com/"
	c.OIDC.ClientID = "oagent"
}

func TestLoad_OIDCDefaults(t *testing.T) {
	c := validConfig(t)
	assert.False(t, c.OIDC.Enabled())
	assert.Equal(t, []string{"openid", "profile", "email"}, c.OIDC.Scopes)
	assert.Equal(t, "Continue with SSO", c.OIDC.ButtonLabel)
}

func TestLoad_OIDCFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_OIDC_ISSUER", "https://idp.example.com/")
	t.Setenv("OAGENT_OIDC_CLIENT_ID", "oagent")
	t.Setenv("OAGENT_OIDC_CLIENT_SECRET", "shh")
	t.Setenv("OAGENT_OIDC_SCOPES", "openid, email")
	t.Setenv("OAGENT_OIDC_BUTTON_LABEL", "Continue with Acme")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.OIDC.Enabled())
	assert.Equal(t, "https://idp.example.com", c.OIDC.NormalizedIssuer())
	assert.Equal(t, "oagent", c.OIDC.ClientID)
	assert.Equal(t, "shh", c.OIDC.ClientSecret)
	assert.Equal(t, []string{"openid", "email"}, c.OIDC.Scopes)
	assert.Equal(t, "Continue with Acme", c.OIDC.ButtonLabel)
}
