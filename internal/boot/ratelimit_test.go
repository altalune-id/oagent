package boot

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/ratelimit"
	"altalune.id/oagent/ratelimit/memory"
)

func TestBuildLimiters_OffByDefault(t *testing.T) {
	l, err := buildLimiters(config.Config{}, testLogger())
	require.NoError(t, err)
	assert.Equal(t, ratelimit.Disabled(), l.auth)
	assert.Nil(t, l.janitor, "no store, no janitor")
}

func TestBuildLimiters_AuthOnWithMemory(t *testing.T) {
	for _, store := range []config.RateLimitStore{"", config.RateLimitStoreMemory} {
		cfg := config.Config{RateLimit: config.RateLimitConfig{Store: store, MaxKeys: 7, Auth: config.RateLimitGroupConfig{Enabled: true}}}
		l, err := buildLimiters(cfg, testLogger())
		require.NoError(t, err)
		m, ok := l.auth.(*memory.Store)
		require.True(t, ok, "got %T", l.auth)
		assert.Same(t, m, l.janitor, "the janitor sweeps the same store")
	}
}

func TestBuildLimiters_UnknownStore(t *testing.T) {
	cfg := config.Config{RateLimit: config.RateLimitConfig{Store: config.RateLimitStoreRedis, Auth: config.RateLimitGroupConfig{Enabled: true}}}
	_, err := buildLimiters(cfg, testLogger())
	assert.ErrorContains(t, err, "rateLimit.store")
}

func TestBoot_RateLimitAuthEnabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{
		DB:        config.DBConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{Store: config.RateLimitStoreMemory, MaxKeys: 10, Auth: config.RateLimitGroupConfig{Enabled: true}},
	}, testLogger())
	require.NoError(t, err)
	assert.Contains(t, workerNames(app), "ratelimit.janitor")
	assert.Len(t, app.Supervisor.Workers(), 6) // the default five + ratelimit.janitor
	assert.IsType(t, &memory.Store{}, app.Services.AuthLimiter)
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_RateLimitStoreError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, err := Boot(context.Background(), config.Config{
		DB:        config.DBConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{Store: "pigeon", Auth: config.RateLimitGroupConfig{Enabled: true}},
	}, testLogger())
	assert.ErrorContains(t, err, "rateLimit.store")
}

func TestAuthLimits(t *testing.T) {
	l := authLimits(config.Config{}, &Services{AuthLimiter: ratelimit.Disabled()})
	assert.Equal(t, ratelimit.Disabled(), l.Limiter)
	assert.Zero(t, l.TrustedProxyHops)

	m := memory.New(1)
	l = authLimits(config.Config{HTTP: config.HTTPConfig{TrustedProxyHops: 2}}, &Services{AuthLimiter: m})
	assert.Same(t, m, l.Limiter)
	assert.Equal(t, 2, l.TrustedProxyHops)
}
