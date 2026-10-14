//go:build integration

package boot

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/testutil/redistest"
	"altalune.id/oagent/ratelimit"
)

func TestOpenRedis_RealServer(t *testing.T) {
	h := redistest.New(t)
	c, err := openRedis(t.Context(), redisTestConfig(h.URL), nooptrace.NewTracerProvider())
	require.NoError(t, err)
	require.NoError(t, c.Set(t.Context(), h.Prefix+"k", "v", time.Minute).Err())
	assert.Equal(t, "v", c.Get(t.Context(), h.Prefix+"k").Val())
	require.NoError(t, c.Del(t.Context(), h.Prefix+"k").Err())

	rh := newRedisHealth(c, time.Second, testLogger())
	rh.probe(t.Context())
	assert.True(t, rh.Ready())

	require.NoError(t, c.Close())
	assert.NotPanics(t, func() { assert.ErrorIs(t, c.Close(), redis.ErrClosed) }, "a second Close is an error, not a panic")
	rh.probe(t.Context())
	assert.False(t, rh.Ready())
}

func TestBoot_RedisFeatureOpensTheRealServer(t *testing.T) {
	restoreSeams(t)
	h := redistest.New(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	cfg := redisFeatureConfig()
	cfg.Redis = redisTestConfig(h.URL)
	cfg.Mail = config.MailConfig{Driver: config.MailDriverSMTP}

	_, err := Boot(t.Context(), cfg, testLogger())
	assert.ErrorContains(t, err, "mail", "boot reached the mailer, so Redis connected")
}

// Review Focus 5: boot writes rate limit keys under redis.keyPrefix + "rl:", each with a TTL.
func TestBuildLimiters_RedisKeysUnderPrefix(t *testing.T) {
	h := redistest.New(t)
	rdb := h.Client(t)
	cfg := redisFeatureConfig()
	cfg.Redis.KeyPrefix = h.Prefix
	l, err := buildLimiters(cfg, rdb, testLogger())
	require.NoError(t, err)

	rule := ratelimit.Rule{Name: "login-ip", Rate: 60, Per: 15 * time.Minute, Burst: 60}
	_, err = l.auth.Allow(t.Context(), ratelimit.Key(rule, "ip:192.0.2.1"), rule)
	require.NoError(t, err)
	keys, err := rdb.Keys(t.Context(), h.Prefix+"*").Result()
	require.NoError(t, err)
	assert.Equal(t, []string{h.Prefix + "rl:login-ip:ip:192.0.2.1"}, keys)
	ttl, err := rdb.PTTL(t.Context(), keys[0]).Result()
	require.NoError(t, err)
	assert.Positive(t, ttl)
}
