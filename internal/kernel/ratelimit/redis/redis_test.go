package redis_test

import (
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/ratelimittest"
	ratelimitredis "altalune.id/oagent/internal/kernel/ratelimit/redis"
)

func closedClient(t *testing.T) *goredis.Client {
	t.Helper()
	c := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, c.Close())
	return c
}

// Review Focus 3: a Redis that fails is an error, never an allowed call, so auth fails closed.
func TestStore_ClosedClientIsAnError(t *testing.T) {
	s := ratelimitredis.New(closedClient(t), "oagent:rl:")
	res, err := s.Allow(t.Context(), "k", ratelimittest.Rule())
	require.Error(t, err)
	assert.ErrorIs(t, err, goredis.ErrClosed)
	assert.False(t, res.Allowed)
	assert.ErrorContains(t, err, "ratelimit/redis")

	err = s.Reset(t.Context(), "k")
	assert.ErrorIs(t, err, goredis.ErrClosed)
}

func TestStore_InvalidRuleNeverReachesRedis(t *testing.T) {
	s := ratelimitredis.New(closedClient(t), "oagent:rl:")
	_, err := s.Allow(t.Context(), "k", ratelimit.Rule{Name: "bad"})
	assert.True(t, ratelimit.IsInvalidRuleError(err), "got %v", err)
}

func TestStore_RejectsSubMillisecondInterval(t *testing.T) {
	s := ratelimitredis.New(closedClient(t), "oagent:rl:")
	_, err := s.Allow(t.Context(), "k", ratelimit.Rule{Name: "fast", Rate: 5000, Per: time.Second, Burst: 1})
	assert.True(t, ratelimit.IsInvalidRuleError(err), "got %v", err)
}
