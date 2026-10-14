// Package ratelimittest is the contract test every ratelimit store runs.
package ratelimittest

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/ratelimit"
)

// Clock is a fake clock the contract moves forward; a store under test reads Now.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a Clock stopped at a fixed instant.
func NewClock() *Clock {
	return &Clock{now: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
}

// Now returns the clock's current instant.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Rule is the contract's rule: 10 per 15 minutes with a burst of 5, so one more every 90 seconds.
func Rule() ratelimit.Rule {
	return ratelimit.Rule{Name: "contract", Rate: 10, Per: 15 * time.Minute, Burst: 5}
}

// Run checks the Limiter contract against fresh limiters from newLimiter, each reading clock.
func Run(t *testing.T, newLimiter func(t *testing.T, clock *Clock) ratelimit.Limiter) {
	t.Helper()
	rule := Rule()
	t.Run("burst then one per interval", func(t *testing.T) {
		clock := NewClock()
		l := newLimiter(t, clock)
		key := ratelimit.Key(rule, "ip:192.0.2.1")
		for i := range rule.Burst {
			res, err := l.Allow(t.Context(), key, rule)
			require.NoError(t, err)
			require.True(t, res.Allowed, "call %d", i+1)
			assert.Equal(t, rule.Burst-1-i, res.Remaining, "call %d", i+1)
		}
		res, err := l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		assert.False(t, res.Allowed)
		assert.Equal(t, 90*time.Second, res.RetryAfter)
		assert.Zero(t, res.Remaining)

		clock.Advance(89 * time.Second)
		res, err = l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		assert.False(t, res.Allowed)
		assert.Equal(t, time.Second, res.RetryAfter)

		clock.Advance(time.Second)
		res, err = l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		res, err = l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		assert.False(t, res.Allowed)
	})

	t.Run("keys do not interfere", func(t *testing.T) {
		l := newLimiter(t, NewClock())
		a, b := ratelimit.Key(rule, "ip:192.0.2.1"), ratelimit.Key(rule, "ip:192.0.2.2")
		drain(t, l, a)
		res, err := l.Allow(t.Context(), b, rule)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
	})

	t.Run("reset clears a key", func(t *testing.T) {
		l := newLimiter(t, NewClock())
		key := ratelimit.Key(rule, "email:x")
		drain(t, l, key)
		require.NoError(t, l.Reset(t.Context(), key))
		res, err := l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		assert.True(t, res.Allowed)
		assert.Equal(t, rule.Burst-1, res.Remaining)
		require.NoError(t, l.Reset(t.Context(), ratelimit.Key(rule, "never-seen")))
	})

	t.Run("concurrent calls allow exactly the burst", func(t *testing.T) {
		l := newLimiter(t, NewClock())
		key := ratelimit.Key(rule, "ip:198.51.100.7")
		var allowed atomic.Int64
		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				res, err := l.Allow(t.Context(), key, rule)
				assert.NoError(t, err)
				if res.Allowed {
					allowed.Add(1)
				}
			})
		}
		wg.Wait()
		assert.Equal(t, int64(rule.Burst), allowed.Load())
	})

	t.Run("an invalid rule is an error", func(t *testing.T) {
		l := newLimiter(t, NewClock())
		_, err := l.Allow(t.Context(), "k", ratelimit.Rule{Name: "bad"})
		assert.True(t, ratelimit.IsInvalidRuleError(err), "got %v", err)
	})
}

func drain(t *testing.T, l ratelimit.Limiter, key string) {
	t.Helper()
	rule := Rule()
	for range rule.Burst {
		res, err := l.Allow(t.Context(), key, rule)
		require.NoError(t, err)
		require.True(t, res.Allowed)
	}
	res, err := l.Allow(t.Context(), key, rule)
	require.NoError(t, err)
	require.False(t, res.Allowed)
}
