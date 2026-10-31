package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpOAuthOn(c *Config) {
	oidcOn(c)
	c.MCP.Enabled = true
}

func TestLoad_MCPDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.MCP.Audience)
	assert.False(t, c.MCP.AudienceOverride)
	assert.Equal(t, time.Minute, c.MCP.ClockSkew)
	assert.False(t, c.MCP.OAuth(c.OIDC))
}

func TestLoad_MCPFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_MCP_AUDIENCE", "https://proxy.example.com/mcp")
	t.Setenv("OAGENT_MCP_AUDIENCE_OVERRIDE", "true")
	t.Setenv("OAGENT_MCP_CLOCK_SKEW", "45s")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "https://proxy.example.com/mcp", c.MCP.Audience)
	assert.True(t, c.MCP.AudienceOverride)
	assert.Equal(t, 45*time.Second, c.MCP.ClockSkew)
}

func TestMCPConfig_OAuth(t *testing.T) {
	on := OIDCConfig{Issuer: "https://idp.example.com"}
	assert.True(t, MCPConfig{Enabled: true}.OAuth(on))
	assert.False(t, MCPConfig{Enabled: false}.OAuth(on))
	assert.False(t, MCPConfig{Enabled: true}.OAuth(OIDCConfig{}))
}

func TestMountedMCP(t *testing.T) {
	assert.Equal(t, "https://oagent.example.com/mcp", MountedMCP(HTTPConfig{BaseURL: "https://oagent.example.com/"}))
	assert.Equal(t, "https://oagent.example.com/oagent/mcp", MountedMCP(HTTPConfig{BaseURL: "https://oagent.example.com", BasePath: "/oagent/"}))
	assert.Empty(t, MountedMCP(HTTPConfig{}))
}
