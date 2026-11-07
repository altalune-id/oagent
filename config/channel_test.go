package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ChannelDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.Channel.DefaultOpenwaBaseURL, "no default OpenWA URL unless the operator sets one")
	assert.Equal(t, 30*time.Minute, c.Channel.MaxMessageAge)
	assert.False(t, c.Channel.AllowPrivateHosts, "production reaches only public OpenWA hosts unless the operator says so")
	assert.True(t, c.RateLimit.Ingest.Enabled, "a default deploy limits inbound deliveries per connection")
	assert.Equal(t, 120, c.RateLimit.Ingest.PerConnectionPerMinute)
	assert.Equal(t, 30, c.RateLimit.Ingest.Burst)
	assert.Equal(t, 600, c.RateLimit.Ingest.PerIPPerMinute, "every /hooks delivery counts per client IP before its connection is looked up")
}

func TestLoad_ChannelFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_CHANNEL_DEFAULT_OPENWA_BASE_URL", "https://wa.example.com")
	t.Setenv("OAGENT_CHANNEL_MAX_MESSAGE_AGE", "10m")
	t.Setenv("OAGENT_RATE_LIMIT_INGEST_ENABLED", "false")
	t.Setenv("OAGENT_RATE_LIMIT_INGEST_PER_CONNECTION_PER_MINUTE", "60")
	t.Setenv("OAGENT_RATE_LIMIT_INGEST_BURST", "10")
	t.Setenv("OAGENT_RATE_LIMIT_INGEST_PER_IP_PER_MINUTE", "90")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "https://wa.example.com", c.Channel.DefaultOpenwaBaseURL)
	assert.Equal(t, 10*time.Minute, c.Channel.MaxMessageAge)
	assert.False(t, c.RateLimit.Ingest.Enabled)
	assert.Equal(t, 60, c.RateLimit.Ingest.PerConnectionPerMinute)
	assert.Equal(t, 10, c.RateLimit.Ingest.Burst)
	assert.Equal(t, 90, c.RateLimit.Ingest.PerIPPerMinute)
}

func TestRateLimit_IngestAloneNeedsADriver(t *testing.T) {
	c := validConfig(t)
	c.RateLimit.Auth.Enabled, c.RateLimit.DataPlane.Enabled = false, false
	assert.True(t, c.RateLimit.AnyEnabled())
	c.RateLimit.Ingest.Enabled = false
	assert.False(t, c.RateLimit.AnyEnabled())
}
