// Package memory is the in-process ratelimit store: GCRA over a bounded map, so limits are per instance.
package memory

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"altalune.id/oagent/internal/kernel/ratelimit"
)

// DefaultMaxKeys bounds the map when New gets no positive maxKeys.
const DefaultMaxKeys = 100_000

const (
	sweepEvery     = time.Minute
	fullSweepEvery = time.Second
	fullRetry      = time.Minute
)

var _ ratelimit.Limiter = (*Store)(nil)

// Store keeps one theoretical arrival time per key; it is also the worker that sweeps refilled keys.
type Store struct {
	maxKeys int
	now     func() time.Time
	log     *slog.Logger

	mu        sync.Mutex
	tat       map[string]time.Time
	fullLogs  time.Time
	fullSweep time.Time
}

// Option customizes New.
type Option func(*Store)

// WithClock sets the store's clock.
func WithClock(now func() time.Time) Option { return func(s *Store) { s.now = now } }

// WithLogger sets the logger that reports a full store.
func WithLogger(log *slog.Logger) Option {
	return func(s *Store) {
		if log != nil {
			s.log = log
		}
	}
}

// New returns a Store that holds at most maxKeys keys.
func New(maxKeys int, opts ...Option) *Store {
	s := &Store{maxKeys: maxKeys, now: time.Now, log: slog.Default(), tat: make(map[string]time.Time)}
	if s.maxKeys <= 0 {
		s.maxKeys = DefaultMaxKeys
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Allow implements ratelimit.Limiter. SECURITY: when full, it sweeps at most once a second, then refuses a new key rather than let it in.
func (s *Store) Allow(ctx context.Context, key string, r ratelimit.Rule) (ratelimit.Result, error) {
	if err := r.Validate(); err != nil {
		return ratelimit.Result{}, err
	}
	interval, tolerance := r.Interval(), r.Tolerance()
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.tat[key]
	if !seen && len(s.tat) >= s.maxKeys && !now.Before(s.fullSweep) {
		s.fullSweep = now.Add(fullSweepEvery)
		s.sweep(now)
	}
	if !seen && len(s.tat) >= s.maxKeys {
		s.logFull(ctx, now)
		return ratelimit.Result{RetryAfter: fullRetry}, nil
	}
	tat := now
	if last.After(now) {
		tat = last
	}
	if allowAt := tat.Add(-tolerance); now.Before(allowAt) {
		return ratelimit.Result{RetryAfter: allowAt.Sub(now)}, nil
	}
	next := tat.Add(interval)
	s.tat[key] = next
	return ratelimit.Result{Allowed: true, Remaining: int((now.Add(tolerance + interval).Sub(next)) / interval)}, nil
}

// Reset implements ratelimit.Limiter.
func (s *Store) Reset(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tat, key)
	return nil
}

// Sweep removes every key that has fully refilled and returns how many it removed.
func (s *Store) Sweep() int {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweep(now)
}

func (s *Store) sweep(now time.Time) int {
	n := 0
	for k, t := range s.tat {
		if !t.After(now) {
			delete(s.tat, k)
			n++
		}
	}
	return n
}

// Len returns how many keys the store holds.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tat)
}

// Name implements worker.Worker.
func (s *Store) Name() string { return "ratelimit.janitor" }

// Run implements worker.Worker: it sweeps every minute until ctx is done.
func (s *Store) Run(ctx context.Context) error {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.Sweep()
		}
	}
}

func (s *Store) logFull(ctx context.Context, now time.Time) {
	if now.Before(s.fullLogs) {
		return
	}
	s.fullLogs = now.Add(time.Minute)
	s.log.WarnContext(ctx, "ratelimit: store full; refusing new keys", slog.Int("max_keys", s.maxKeys))
}
