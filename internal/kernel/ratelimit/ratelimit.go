// Package ratelimit defines the Limiter contract that the memory and Redis stores implement.
package ratelimit

import (
	"context"
	"time"
)

// Limiter counts calls per key under a Rule.
type Limiter interface {
	// Allow counts one call for key and reports whether it may go ahead.
	Allow(ctx context.Context, key string, r Rule) (Result, error)
	// Reset forgets key, as if it had never been seen.
	Reset(ctx context.Context, key string) error
}

// Rule allows Rate calls every Per, up to Burst at once (GCRA).
type Rule struct {
	Name  string
	Rate  int
	Per   time.Duration
	Burst int
}

// Interval is the time one call takes to refill.
func (r Rule) Interval() time.Duration { return r.Per / time.Duration(r.Rate) }

// Tolerance is how far ahead of now a key's next allowed time may run before calls are refused.
func (r Rule) Tolerance() time.Duration { return r.Interval() * time.Duration(r.Burst-1) }

// Validate rejects a rule a store cannot count.
func (r Rule) Validate() error {
	switch {
	case r.Name == "":
		return &InvalidRuleError{Rule: r.Name, Reason: "name required"}
	case r.Rate <= 0 || r.Per <= 0:
		return &InvalidRuleError{Rule: r.Name, Reason: "rate and period must be positive"}
	case r.Burst < 1:
		return &InvalidRuleError{Rule: r.Name, Reason: "burst must be at least 1"}
	case r.Interval() <= 0:
		return &InvalidRuleError{Rule: r.Name, Reason: "rate is too high for the period"}
	}
	return nil
}

// Result is the outcome of one Allow call.
type Result struct {
	Allowed    bool
	RetryAfter time.Duration
	Remaining  int
}

// RetryAfterSeconds is RetryAfter rounded up to whole seconds, for a Retry-After header.
func (r Result) RetryAfterSeconds() int {
	if r.RetryAfter <= 0 {
		return 0
	}
	return int((r.RetryAfter + time.Second - 1) / time.Second)
}
