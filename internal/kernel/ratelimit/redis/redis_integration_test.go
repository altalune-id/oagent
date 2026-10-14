//go:build integration

package redis_test

import (
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/kernel/ratelimit/ratelimittest"
	ratelimitredis "altalune.id/oagent/internal/kernel/ratelimit/redis"
	"altalune.id/oagent/internal/testutil/redistest"
)

func TestStore_Contract(t *testing.T) {
	ratelimittest.Run(t, func(t *testing.T, clock *ratelimittest.Clock) ratelimit.Limiter {
		h := redistest.New(t)
		return ratelimitredis.New(h.Client(t), h.Prefix+"rl:", ratelimitredis.WithClock(clock.Now))
	})
}

func TestStore_RedisClock(t *testing.T) {
	h := redistest.New(t)
	s := ratelimitredis.New(h.Client(t), h.Prefix+"rl:")
	rule := ratelimittest.Rule()
	key := ratelimit.Key(rule, "ip:192.0.2.9")
	for i := range rule.Burst {
		res, err := s.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		require.True(t, res.Allowed, "call %d", i+1)
		assert.Equal(t, rule.Burst-1-i, res.Remaining)
	}
	res, err := s.Allow(t.Context(), key, rule)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Greater(t, res.RetryAfter, 85*time.Second)
	assert.LessOrEqual(t, res.RetryAfter, 90*time.Second)
}

// Review Focus 2: two servers on one Redis and prefix share one budget; two memory stores do not.
func TestStore_SharedAcrossServers(t *testing.T) {
	h := redistest.New(t)
	rule := ratelimittest.Rule()
	key := ratelimit.Key(rule, "email:shared")
	clock := ratelimittest.NewClock()
	a := ratelimitredis.New(h.Client(t), h.Prefix+"rl:", ratelimitredis.WithClock(clock.Now))
	b := ratelimitredis.New(h.Client(t), h.Prefix+"rl:", ratelimitredis.WithClock(clock.Now))
	assert.Equal(t, rule.Burst, allowed(t, rule, key, a, b, 2*rule.Burst), "the servers share one burst")

	m1, m2 := memory.New(10, memory.WithClock(clock.Now)), memory.New(10, memory.WithClock(clock.Now))
	assert.Equal(t, 2*rule.Burst, allowed(t, rule, key, m1, m2, 2*rule.Burst), "memory counts per server")
}

func allowed(t *testing.T, rule ratelimit.Rule, key string, a, b ratelimit.Limiter, calls int) int {
	t.Helper()
	n := 0
	for i := range calls * 2 {
		l := a
		if i%2 == 1 {
			l = b
		}
		res, err := l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		if res.Allowed {
			n++
		}
	}
	return n
}

// Review Focus 5: every key sits under the prefix and rl:, carries a TTL, and another prefix never sees it.
func TestStore_KeyHygiene(t *testing.T) {
	h := redistest.New(t)
	other := redistest.New(t)
	rdb := h.Client(t)
	rule := ratelimittest.Rule()
	key := ratelimit.Key(rule, "ip:203.0.113.5")
	clock := ratelimittest.NewClock()

	prod := ratelimitredis.New(rdb, h.Prefix+"rl:", ratelimitredis.WithClock(clock.Now))
	staging := ratelimitredis.New(other.Client(t), other.Prefix+"rl:", ratelimitredis.WithClock(clock.Now))
	for range rule.Burst + 1 {
		_, err := prod.Allow(t.Context(), key, rule)
		require.NoError(t, err)
	}
	res, err := staging.Allow(t.Context(), key, rule)
	require.NoError(t, err)
	assert.True(t, res.Allowed, "another prefix keeps its own count")
	assert.Equal(t, rule.Burst-1, res.Remaining)

	keys, err := rdb.Keys(t.Context(), h.Prefix+"*").Result()
	require.NoError(t, err)
	require.Equal(t, []string{h.Prefix + "rl:" + key}, keys)
	ttl, err := rdb.PTTL(t.Context(), keys[0]).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "the key expires by itself")
	assert.LessOrEqual(t, ttl, rule.Interval()*time.Duration(rule.Burst)+time.Second, "no longer than a full refill")

	require.NoError(t, prod.Reset(t.Context(), key))
	n, err := rdb.Exists(t.Context(), keys[0]).Result()
	require.NoError(t, err)
	assert.Zero(t, n, "Reset deletes the key")
}

func TestStore_ClosedRealClient(t *testing.T) {
	h := redistest.New(t)
	opts, err := goredis.ParseURL(h.URL)
	require.NoError(t, err)
	c := goredis.NewClient(opts)
	s := ratelimitredis.New(c, h.Prefix+"rl:")
	_, err = s.Allow(t.Context(), "k", ratelimittest.Rule())
	require.NoError(t, err)
	require.NoError(t, c.Del(t.Context(), h.Prefix+"rl:k").Err())
	require.NoError(t, c.Close())
	_, err = s.Allow(t.Context(), "k", ratelimittest.Rule())
	assert.Error(t, err)
}
