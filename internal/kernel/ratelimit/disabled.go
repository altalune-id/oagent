package ratelimit

import "context"

// Disabled returns a Limiter that allows every call and never fails.
func Disabled() Limiter { return disabled{} }

type disabled struct{}

func (disabled) Allow(_ context.Context, _ string, r Rule) (Result, error) {
	return Result{Allowed: true, Remaining: r.Burst}, nil
}

func (disabled) Reset(context.Context, string) error { return nil }
