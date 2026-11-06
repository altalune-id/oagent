package dataplane

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/web/layout"
)

// Limits counts data-plane calls per API key and per client IP; the zero value limits nothing.
type Limits struct {
	Limiter          ratelimit.Limiter
	PerKey, PerIP    ratelimit.Rule
	TrustedProxyHops int
}

// KeyRule limits one API key to perMinute data-plane calls a minute, all at once at most.
func KeyRule(perMinute int) ratelimit.Rule {
	return ratelimit.Rule{Name: "dataplane-key", Rate: perMinute, Per: time.Minute, Burst: perMinute}
}

// IPRule limits one client IP to perMinute data-plane calls a minute, all at once at most.
func IPRule(perMinute int) ratelimit.Rule {
	return ratelimit.Rule{Name: "dataplane-ip", Rate: perMinute, Per: time.Minute, Burst: perMinute}
}

func (l Limits) ip(r *http.Request) error {
	return l.allow(r.Context(), l.PerIP, ratelimit.IPSubject(layout.ClientIP(r, l.TrustedProxyHops)))
}

func (l Limits) key(ctx context.Context, keyID string) error {
	return l.allow(ctx, l.PerKey, "key:"+keyID)
}

// SECURITY: a limiter that cannot count fails closed with a 503, never letting the call through.
func (l Limits) allow(ctx context.Context, rule ratelimit.Rule, subject string) error {
	if l.Limiter == nil || rule.Name == "" {
		return nil
	}
	res, err := l.Limiter.Allow(ctx, ratelimit.Key(rule, subject), rule)
	if err != nil {
		return withStatus(http.StatusServiceUnavailable, fmt.Errorf("dataplane: rate limit %s: %w", rule.Name, err))
	}
	if !res.Allowed {
		return tooMany(res.RetryAfter, errTooMany)
	}
	return nil
}

func tooMany(after time.Duration, err error) error {
	return &statusError{status: http.StatusTooManyRequests, retryAfter: max(after, time.Second), err: err}
}

var errTooMany = apperror.New(apperror.CodeRunRateLimited, "Too many requests; try again shortly")
