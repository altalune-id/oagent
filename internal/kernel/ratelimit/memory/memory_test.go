package memory

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/ratelimittest"
)

func TestContract(t *testing.T) {
	ratelimittest.Run(t, func(_ *testing.T, clock *ratelimittest.Clock) ratelimit.Limiter {
		return New(1000, WithClock(clock.Now))
	})
}

func TestStore_FullDeniesNewKeysAndLogsOncePerMinute(t *testing.T) {
	clock := ratelimittest.NewClock()
	var logs bytes.Buffer
	s := New(3, WithClock(clock.Now), WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	rule := ratelimittest.Rule()
	for i := range 3 {
		res, err := s.Allow(t.Context(), ratelimit.Key(rule, string(rune('a'+i))), rule)
		require.NoError(t, err)
		require.True(t, res.Allowed)
	}
	for range 1000 {
		res, err := s.Allow(t.Context(), ratelimit.Key(rule, "flood-"+time.Now().String()), rule)
		require.NoError(t, err)
		assert.False(t, res.Allowed)
		assert.Equal(t, time.Minute, res.RetryAfter)
	}
	assert.Equal(t, 3, s.Len(), "the map never grows past maxKeys")
	res, err := s.Allow(t.Context(), ratelimit.Key(rule, "a"), rule)
	require.NoError(t, err)
	assert.True(t, res.Allowed, "a known key still counts while the store is full")
	assert.Equal(t, 1, strings.Count(logs.String(), "ratelimit: store full"))

	clock.Advance(time.Minute)
	_, _ = s.Allow(t.Context(), ratelimit.Key(rule, "late"), rule)
	assert.Equal(t, 2, strings.Count(logs.String(), "ratelimit: store full"))
	assert.NotContains(t, logs.String(), "late", "keys are never logged")
}

func TestStore_FullSweepsRefilledKeysBeforeRefusing(t *testing.T) {
	clock := ratelimittest.NewClock()
	s := New(3, WithClock(clock.Now))
	rule := ratelimittest.Rule()
	for _, k := range []string{"a", "b", "c"} {
		res, err := s.Allow(t.Context(), k, rule)
		require.NoError(t, err)
		require.True(t, res.Allowed)
	}
	res, err := s.Allow(t.Context(), "live", rule)
	require.NoError(t, err)
	assert.False(t, res.Allowed, "every key is still live")

	clock.Advance(rule.Interval())
	res, err = s.Allow(t.Context(), "new", rule)
	require.NoError(t, err)
	assert.True(t, res.Allowed, "refilled keys make room without waiting for the janitor")
	assert.Equal(t, 1, s.Len())
}

func TestStore_FullSweepsAtMostOncePerSecond(t *testing.T) {
	clock := ratelimittest.NewClock()
	s := New(3, WithClock(clock.Now))
	rule := ratelimit.Rule{Name: "fast", Rate: 2, Per: time.Second, Burst: 1}
	for _, k := range []string{"a", "b", "c"} {
		res, err := s.Allow(t.Context(), k, rule)
		require.NoError(t, err)
		require.True(t, res.Allowed)
	}
	res, err := s.Allow(t.Context(), "x", rule)
	require.NoError(t, err)
	require.False(t, res.Allowed)

	clock.Advance(600 * time.Millisecond)
	res, err = s.Allow(t.Context(), "y", rule)
	require.NoError(t, err)
	assert.False(t, res.Allowed, "no second sweep within a second, though the keys refilled")
	assert.Equal(t, 3, s.Len())

	clock.Advance(400 * time.Millisecond)
	res, err = s.Allow(t.Context(), "z", rule)
	require.NoError(t, err)
	assert.True(t, res.Allowed)
}

func TestStore_SweepRemovesRefilledKeys(t *testing.T) {
	clock := ratelimittest.NewClock()
	s := New(10, WithClock(clock.Now))
	rule := ratelimittest.Rule()
	_, err := s.Allow(t.Context(), "short", rule)
	require.NoError(t, err)
	for range rule.Burst {
		_, err = s.Allow(t.Context(), "long", rule)
		require.NoError(t, err)
	}
	clock.Advance(rule.Interval())
	assert.Equal(t, 1, s.Sweep())
	assert.Equal(t, 1, s.Len())
	clock.Advance(time.Duration(rule.Burst) * rule.Interval())
	assert.Equal(t, 1, s.Sweep())
	assert.Zero(t, s.Len())
}

func TestStore_RunSweepsEveryMinuteUntilCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New(10)
		assert.Equal(t, "ratelimit.janitor", s.Name())
		_, err := s.Allow(t.Context(), "k", ratelimittest.Rule())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- s.Run(ctx) }()

		time.Sleep(ratelimittest.Rule().Interval() + time.Minute)
		synctest.Wait()
		assert.Zero(t, s.Len())

		cancel()
		assert.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestNew_Defaults(t *testing.T) {
	s := New(0, WithLogger(nil))
	assert.Equal(t, DefaultMaxKeys, s.maxKeys)
	assert.NotNil(t, s.log)
}
