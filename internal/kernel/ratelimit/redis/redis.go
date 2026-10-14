// Package redis is the shared ratelimit store: GCRA in one Lua script on Redis, so every server counts against one limit.
package redis

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"altalune.id/oagent/internal/kernel/ratelimit"
)

//go:embed gcra.lua
var gcraSource string

//nolint:gochecknoglobals // the script is immutable; go-redis caches its SHA.
var gcra = goredis.NewScript(gcraSource)

const redisClock = -1

var _ ratelimit.Limiter = (*Store)(nil)

// Store keeps one theoretical arrival time per key in Redis, in milliseconds, under its prefix.
type Store struct {
	client goredis.UniversalClient
	prefix string
	now    func() time.Time
}

// Option customizes New.
type Option func(*Store)

// WithClock makes the store send now instead of using Redis TIME; for tests only.
func WithClock(now func() time.Time) Option { return func(s *Store) { s.now = now } }

// New returns a Store that writes every key as prefix + key.
func New(client goredis.UniversalClient, prefix string, opts ...Option) *Store {
	s := &Store{client: client, prefix: prefix}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Allow implements ratelimit.Limiter. SECURITY: a Redis error or timeout is returned, never an allowed call.
func (s *Store) Allow(ctx context.Context, key string, r ratelimit.Rule) (ratelimit.Result, error) {
	if err := r.Validate(); err != nil {
		return ratelimit.Result{}, err
	}
	interval := r.Interval()
	if interval < time.Millisecond {
		return ratelimit.Result{}, &ratelimit.InvalidRuleError{Rule: r.Name, Reason: "the redis store needs an interval of at least 1ms"}
	}
	now := int64(redisClock)
	if s.now != nil {
		now = s.now().UnixMilli()
	}
	out, err := gcra.Run(ctx, s.client, []string{s.prefix + key},
		interval.Milliseconds(), r.Tolerance().Milliseconds(), now).Int64Slice()
	if err != nil {
		return ratelimit.Result{}, fmt.Errorf("ratelimit/redis: allow: %w", err)
	}
	if len(out) != 3 {
		return ratelimit.Result{}, fmt.Errorf("ratelimit/redis: allow: script returned %d values, want 3", len(out))
	}
	return ratelimit.Result{
		Allowed:    out[0] == 1,
		RetryAfter: time.Duration(out[1]) * time.Millisecond,
		Remaining:  int(out[2]),
	}, nil
}

// Reset implements ratelimit.Limiter.
func (s *Store) Reset(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.prefix+key).Err(); err != nil {
		return fmt.Errorf("ratelimit/redis: reset: %w", err)
	}
	return nil
}
