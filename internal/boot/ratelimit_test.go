package boot

import (
	"context"
	"log/slog"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	ratelimitredis "altalune.id/oagent/internal/kernel/ratelimit/redis"
)

func TestBuildLimiters_OffByDefault(t *testing.T) {
	l, err := buildLimiters(config.Config{}, nil, testLogger())
	require.NoError(t, err)
	assert.Equal(t, ratelimit.Disabled(), l.auth)
	assert.Nil(t, l.janitor, "no store, no janitor")
}

func TestBuildLimiters_AuthOnWithMemory(t *testing.T) {
	for _, driver := range []config.RateLimitDriver{"", config.RateLimitDriverMemory} {
		cfg := config.Config{RateLimit: config.RateLimitConfig{Driver: driver, MaxKeys: 7, Auth: config.RateLimitGroupConfig{Enabled: true}}}
		l, err := buildLimiters(cfg, nil, testLogger())
		require.NoError(t, err)
		m, ok := l.auth.(*memory.Store)
		require.True(t, ok, "got %T", l.auth)
		assert.Same(t, m, l.janitor, "the janitor sweeps the same store")
	}
}

func TestBuildLimiters_UnknownDriver(t *testing.T) {
	cfg := config.Config{RateLimit: config.RateLimitConfig{Driver: "pigeon", Auth: config.RateLimitGroupConfig{Enabled: true}}}
	_, err := buildLimiters(cfg, nil, testLogger())
	assert.ErrorContains(t, err, "rateLimit.driver")
}

func TestBuildLimiters_AuthOnWithRedis(t *testing.T) {
	cfg := redisFeatureConfig()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	l, err := buildLimiters(cfg, client, testLogger())
	require.NoError(t, err)
	assert.IsType(t, &ratelimitredis.Store{}, l.auth)
	assert.Nil(t, l.janitor, "redis keys expire by themselves")
}

func TestBuildLimiters_RedisWithoutConnection(t *testing.T) {
	_, err := buildLimiters(redisFeatureConfig(), nil, testLogger())
	assert.ErrorContains(t, err, "redis")
}

func TestBoot_RateLimitOnRedis(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubRedis(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), redisFeatureConfig(), testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, *calls, "one shared connection")
	assert.IsType(t, &ratelimitredis.Store{}, app.Services.AuthLimiter)
	assert.Contains(t, workerNames(app), "redis-health")
	assert.NotContains(t, workerNames(app), "ratelimit.janitor")
	assert.Len(t, app.Supervisor.Workers(), 6) // the default five + redis-health
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_RateLimitAuthEnabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{
		Postgres:  config.PostgresConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{Driver: config.RateLimitDriverMemory, MaxKeys: 10, Auth: config.RateLimitGroupConfig{Enabled: true}},
	}, testLogger())
	require.NoError(t, err)
	assert.Contains(t, workerNames(app), "ratelimit.janitor")
	assert.Len(t, app.Supervisor.Workers(), 6) // the default five + ratelimit.janitor
	assert.IsType(t, &memory.Store{}, app.Services.AuthLimiter)
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_RateLimitDriverError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, err := Boot(context.Background(), config.Config{
		Postgres:  config.PostgresConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{Driver: "pigeon", Auth: config.RateLimitGroupConfig{Enabled: true}},
	}, testLogger())
	assert.ErrorContains(t, err, "rateLimit.driver")
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
