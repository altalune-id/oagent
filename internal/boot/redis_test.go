package boot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	redisconn "altalune.id/oagent/redis"
)

const redisTestPassword = "hunter2-redis-secret"

func redisTestConfig(url string) config.RedisConfig {
	return config.RedisConfig{
		URL: url, KeyPrefix: "oagent:", MaxConnections: 10, MaxIdleTime: 5 * time.Minute,
		ConnectTimeout: 300 * time.Millisecond, ReadTimeout: time.Second, WriteTimeout: time.Second,
	}
}

func redisFeatureConfig() config.Config {
	return config.Config{
		Postgres: config.PostgresConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{
			Driver: config.RateLimitDriverRedis, Auth: config.RateLimitGroupConfig{Enabled: true},
		},
		Redis: redisTestConfig("redis://:" + redisTestPassword + "@127.0.0.1:1/0"),
	}
}

func TestRedisConfig(t *testing.T) {
	c := redisTestConfig("redis://h:6379/0")
	assert.Equal(t, redisconn.Config{
		URL: "redis://h:6379/0", MaxConnections: 10, MaxIdleTime: 5 * time.Minute,
		ConnectTimeout: 300 * time.Millisecond, ReadTimeout: time.Second, WriteTimeout: time.Second,
	}, redisConfig(c))
}

func stubRedis(t *testing.T) (calls *int, client *redis.Client) {
	t.Helper()
	calls = new(int)
	client = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	openRedisFn = func(context.Context, redisconn.Config, trace.TracerProvider) (redis.UniversalClient, error) {
		*calls++
		return client, nil
	}
	return calls, client
}

// Review Focus 1: with nothing selecting Redis, boot never opens it, even when redis.url is set.
func TestBuildRedis_UnusedNeverConnects(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubRedis(t)
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	k := &kernel.Kernel{}
	rdb, rh, err := buildRedis(t.Context(), k, config.Config{}, nooptrace.NewTracerProvider(), log)
	require.NoError(t, err)
	assert.Nil(t, rdb)
	assert.Nil(t, rh)
	assert.Zero(t, *calls)
	assert.Empty(t, logs.String(), "no url, nothing to say")

	cfg := config.Config{Redis: redisTestConfig("redis://:" + redisTestPassword + "@cache.internal:6379")}
	cfg.RateLimit.Driver = config.RateLimitDriverRedis
	rdb, rh, err = buildRedis(t.Context(), k, cfg, nooptrace.NewTracerProvider(), log)
	require.NoError(t, err)
	assert.Nil(t, rdb)
	assert.Nil(t, rh, "a redis store with every group off needs no connection")
	assert.Zero(t, *calls)
	assert.Contains(t, logs.String(), "no feature uses Redis")
	assert.NotContains(t, logs.String(), redisTestPassword)
	assert.NotContains(t, logs.String(), "cache.internal")
}

func TestBuildRedis_FeatureConnectsOnceAndClosesWithTheKernel(t *testing.T) {
	restoreSeams(t)
	calls, client := stubRedis(t)

	k := &kernel.Kernel{}
	rdb, rh, err := buildRedis(t.Context(), k, redisFeatureConfig(), nooptrace.NewTracerProvider(), testLogger())
	require.NoError(t, err)
	require.NotNil(t, rh)
	assert.Equal(t, 1, *calls)
	assert.Same(t, client, rdb)
	assert.True(t, rh.Ready(), "the boot ping succeeded")
	assert.Equal(t, "redis-health", rh.Name())

	require.NoError(t, k.Close())
	assert.ErrorIs(t, client.Ping(t.Context()).Err(), redis.ErrClosed)
}

func TestBuildRedis_ConnectErrorFailsBoot(t *testing.T) {
	restoreSeams(t)
	openRedisFn = func(context.Context, redisconn.Config, trace.TracerProvider) (redis.UniversalClient, error) {
		return nil, errors.New("redis: cannot connect to 127.0.0.1:1 (redis.url): refused")
	}
	k := &kernel.Kernel{}
	_, _, err := buildRedis(t.Context(), k, redisFeatureConfig(), nooptrace.NewTracerProvider(), testLogger())
	assert.ErrorContains(t, err, "boot: redis: cannot connect")
}

func TestBoot_DefaultConfigNeverOpensRedis(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubRedis(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{Postgres: config.PostgresConfig{DSN: "x"}}, testLogger())
	require.NoError(t, err)
	assert.Zero(t, *calls)
	assert.NotContains(t, workerNames(app), "redis-health")
	assert.Len(t, app.Supervisor.Workers(), 5)
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_LaterFailureClosesRedis(t *testing.T) {
	restoreSeams(t)
	calls, client := stubRedis(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	cfg := redisFeatureConfig()
	cfg.Mail = config.MailConfig{Driver: config.MailDriverSMTP}
	_, err := Boot(context.Background(), cfg, testLogger())
	require.ErrorContains(t, err, "mail")
	assert.Equal(t, 1, *calls)
	assert.ErrorIs(t, client.Ping(t.Context()).Err(), redis.ErrClosed, "a failed boot does not leak the connection")
}

func TestBoot_RedisConnectErrorFailsBoot(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, err := Boot(context.Background(), redisFeatureConfig(), testLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis: cannot connect to 127.0.0.1:1 (redis.url)")
	assert.NotContains(t, err.Error(), redisTestPassword)
}
