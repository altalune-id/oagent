package run

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/ratelimit"
)

// DefaultMaxConcurrentPerProject is how many threads of one project may hold a live lease at once.
const DefaultMaxConcurrentPerProject = 4

// DefaultMaxPerMinutePerProject is how many runs one project may request per minute.
const DefaultMaxPerMinutePerProject = 30

// RateCap limits how often a project may request a run; a nil Limiter or a PerMinute below 1 caps nothing.
type RateCap struct {
	Limiter   ratelimit.Limiter
	PerMinute int
}

// Allow counts one run request for projectID: a *RateLimitedError when over, or the limiter's own error.
func (c RateCap) Allow(ctx context.Context, projectID uuid.UUID) error {
	if c.Limiter == nil || c.PerMinute < 1 {
		return nil
	}
	rule := ratelimit.Rule{Name: "run", Rate: c.PerMinute, Per: time.Minute, Burst: c.PerMinute}
	res, err := c.Limiter.Allow(ctx, ratelimit.Key(rule, projectID.String()), rule)
	if err != nil {
		return err
	}
	if !res.Allowed {
		return &RateLimitedError{RetryAfter: res.RetryAfter}
	}
	return nil
}
