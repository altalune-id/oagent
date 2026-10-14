package boot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
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
		DB: config.DBConfig{DSN: "x"},
		RateLimit: config.RateLimitConfig{
			Store: config.RateLimitStoreRedis, Auth: config.RateLimitGroupConfig{Enabled: true},
		},
		Redis: redisTestConfig("redis://:" + redisTestPassword + "@127.0.0.1:1/0"),
	}
}

func stubRedis(t *testing.T) (calls *int, client *redis.Client) {
	t.Helper()
	calls = new(int)
	client = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	openRedisFn = func(context.Context, config.RedisConfig, trace.TracerProvider) (redis.UniversalClient, error) {
		*calls++
		return client, nil
	}
	return calls, client
}

func silentServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				_ = conn.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func TestOpenRedis_SilentServerFailsWithinConnectTimeout(t *testing.T) {
	addr := silentServer(t)
	start := time.Now()
	c, err := openRedis(t.Context(), redisTestConfig("redis://:"+redisTestPassword+"@"+addr+"/0"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.Nil(t, c)
	assert.Less(t, time.Since(start), 2*time.Second, "bounded by redis.connectTimeout")
	assert.ErrorContains(t, err, "redis: cannot connect to "+addr+" (redis.url)")
	assert.NotContains(t, err.Error(), redisTestPassword)
}

func TestOpenRedis_RefusedHasNoPassword(t *testing.T) {
	_, err := openRedis(t.Context(), redisTestConfig("redis://user:"+redisTestPassword+"@127.0.0.1:1/0"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis: cannot connect to 127.0.0.1:1 (redis.url)")
	assert.NotContains(t, err.Error(), redisTestPassword)
}

func TestOpenRedis_BadURLIsNotEchoed(t *testing.T) {
	_, err := openRedis(t.Context(), redisTestConfig("http://:"+redisTestPassword+"@host:6379"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis.url")
	assert.NotContains(t, err.Error(), redisTestPassword)
	assert.NotContains(t, err.Error(), "host:6379")
}

// Review Focus 1: with nothing selecting Redis, boot never opens it, even when redis.url is set.
func TestBuildRedis_UnusedNeverConnects(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubRedis(t)
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	k := &kernel.Kernel{}
	rh, err := buildRedis(t.Context(), k, config.Config{}, nooptrace.NewTracerProvider(), log)
	require.NoError(t, err)
	assert.Nil(t, rh)
	assert.Zero(t, *calls)
	assert.Empty(t, logs.String(), "no url, nothing to say")

	cfg := config.Config{Redis: redisTestConfig("redis://:" + redisTestPassword + "@cache.internal:6379")}
	cfg.RateLimit.Store = config.RateLimitStoreRedis
	rh, err = buildRedis(t.Context(), k, cfg, nooptrace.NewTracerProvider(), log)
	require.NoError(t, err)
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
	rh, err := buildRedis(t.Context(), k, redisFeatureConfig(), nooptrace.NewTracerProvider(), testLogger())
	require.NoError(t, err)
	require.NotNil(t, rh)
	assert.Equal(t, 1, *calls)
	assert.Same(t, client, rh.client)
	assert.True(t, rh.Ready(), "the boot ping succeeded")
	assert.Equal(t, "redis-health", rh.Name())

	require.NoError(t, k.Close())
	assert.ErrorIs(t, client.Ping(t.Context()).Err(), redis.ErrClosed)
}

func TestBuildRedis_ConnectErrorFailsBoot(t *testing.T) {
	restoreSeams(t)
	openRedisFn = func(context.Context, config.RedisConfig, trace.TracerProvider) (redis.UniversalClient, error) {
		return nil, errors.New("redis: cannot connect to 127.0.0.1:1 (redis.url): refused")
	}
	k := &kernel.Kernel{}
	_, err := buildRedis(t.Context(), k, redisFeatureConfig(), nooptrace.NewTracerProvider(), testLogger())
	assert.ErrorContains(t, err, "boot: redis: cannot connect")
}

func TestReadinessChecks(t *testing.T) {
	dbh := &db.HealthMonitor{}
	assert.Len(t, readinessChecks(dbh, nil), 1, "no redis, no redis check")
	rh := newRedisHealth(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), time.Second, testLogger())
	checks := readinessChecks(dbh, rh)
	require.Len(t, checks, 2)
	assert.Same(t, rh, checks[1])
}

// Review Focus 3: a Redis that stops answering makes the server not ready, and it never crashes the worker.
func TestRedisHealth_ProbeFailureMarksNotReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
		require.NoError(t, client.Close())
		var logs bytes.Buffer
		rh := newRedisHealth(client, time.Second, slog.New(slog.NewTextHandler(&logs, nil)))
		require.True(t, rh.Ready())

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- rh.Run(ctx) }()
		synctest.Wait()
		assert.False(t, rh.Ready())
		time.Sleep(3 * redisProbeEvery)
		synctest.Wait()
		assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("redis: not ready")), "logged once per change, not per probe")

		cancel()
		assert.NoError(t, <-done)
	})
}

func TestBoot_DefaultConfigNeverOpensRedis(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubRedis(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x"}}, testLogger())
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
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
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
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, err := Boot(context.Background(), redisFeatureConfig(), testLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis: cannot connect to 127.0.0.1:1 (redis.url)")
	assert.NotContains(t, err.Error(), redisTestPassword)
}
