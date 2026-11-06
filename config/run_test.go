package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RunDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, 4, c.Run.MaxConcurrentPerProject)
	assert.Equal(t, 30, c.Run.MaxPerMinutePerProject)
}

func TestLoad_RunFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_RUN_MAX_CONCURRENT_PER_PROJECT", "2")
	t.Setenv("OAGENT_RUN_MAX_PER_MINUTE_PER_PROJECT", "10")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, 2, c.Run.MaxConcurrentPerProject)
	assert.Equal(t, 10, c.Run.MaxPerMinutePerProject)
}

func TestValidate_Run(t *testing.T) {
	c := validConfig(t)
	c.Run.MaxConcurrentPerProject = 0
	invalidSetting("run.maxConcurrentPerProject")(t, c.Validate())
	c = validConfig(t)
	c.Run.MaxPerMinutePerProject = 0
	invalidSetting("run.maxPerMinutePerProject")(t, c.Validate())
}

func TestConfig_RunDefaultsAndBounds(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, 10*time.Minute, c.Run.TurnTimeout)
	assert.Equal(t, time.Minute, c.Run.LeaseTTL)
	assert.Equal(t, 20*time.Second, c.Run.Heartbeat)
	assert.Equal(t, 25*time.Second, c.Run.DrainTimeout)
	assert.Equal(t, 15*time.Second, c.Run.SweepEvery)
	assert.Equal(t, 3, c.Run.MaxAttempts)
	assert.Equal(t, 8, c.Run.MaxConcurrentPerInstance)

	for name, c := range map[string]struct {
		mutate func(c *Config)
		key    string
	}{
		"turn timeout over 14m":  {func(c *Config) { c.Run.TurnTimeout = 15 * time.Minute }, "run.turnTimeout"},
		"turn timeout under 1m":  {func(c *Config) { c.Run.TurnTimeout = 30 * time.Second }, "run.turnTimeout"},
		"heartbeat over ttl/2":   {func(c *Config) { c.Run.Heartbeat = 31 * time.Second }, "run.heartbeat"},
		"drain not under grace":  {func(c *Config) { c.Run.DrainTimeout = c.Scheduler.ShutdownGrace }, "run.drainTimeout"},
		"no attempts":            {func(c *Config) { c.Run.MaxAttempts = 0 }, "run.maxAttempts"},
		"no turns per instance":  {func(c *Config) { c.Run.MaxConcurrentPerInstance = 0 }, "run.maxConcurrentPerInstance"},
		"sweep under one second": {func(c *Config) { c.Run.SweepEvery = 500 * time.Millisecond }, "run.sweepEvery"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig(t)
			c.mutate(cfg)
			invalidSetting(c.key)(t, cfg.Validate())
		})
	}
}
