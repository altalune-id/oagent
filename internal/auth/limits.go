package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/web/layout"
)

const limiterDownRetry = 5 * time.Second

const limiterDown = "Sign-in is temporarily unavailable. Try again in a moment."

// LoginEmailRule counts failed sign-ins per email: 10 per 15 minutes, 5 at once.
func LoginEmailRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "login-email", Rate: 10, Per: 15 * time.Minute, Burst: 5}
}

// LoginIPRule counts every sign-in per client IP: 60 per 15 minutes.
func LoginIPRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "login-ip", Rate: 60, Per: 15 * time.Minute, Burst: 60}
}

// Limits rate limits the sign-in forms by client IP and email; the zero value limits nothing.
type Limits struct {
	Limiter          ratelimit.Limiter
	TrustedProxyHops int
}

// LimitIP counts r against rule by its client IP: a *RateLimitedError when over, or the limiter's own error.
func (l Limits) LimitIP(r *http.Request, rule ratelimit.Rule) error {
	return l.allow(r.Context(), ratelimit.Key(rule, ratelimit.IPSubject(layout.ClientIP(r, l.TrustedProxyHops))), rule)
}

// LimitEmail counts one attempt for email against rule; the key holds a hash, not the email.
func (l Limits) LimitEmail(ctx context.Context, email string, rule ratelimit.Rule) error {
	return l.allow(ctx, emailKey(email, rule), rule)
}

// ForgetEmail clears email's count under rule.
func (l Limits) ForgetEmail(ctx context.Context, email string, rule ratelimit.Rule) error {
	return l.limiter().Reset(ctx, emailKey(email, rule))
}

func (l Limits) allow(ctx context.Context, key string, rule ratelimit.Rule) error {
	res, err := l.limiter().Allow(ctx, key, rule)
	if err != nil {
		return err
	}
	if !res.Allowed {
		return &RateLimitedError{RetryAfter: res.RetryAfter}
	}
	return nil
}

func (l Limits) limiter() ratelimit.Limiter {
	if l.Limiter == nil {
		return ratelimit.Disabled()
	}
	return l.Limiter
}

func emailKey(email string, rule ratelimit.Rule) string {
	return ratelimit.Key(rule, ratelimit.EmailSubject(email))
}

// WriteLimited answers a refused form: 429 for a *RateLimitedError, else 503. SECURITY: fails closed and never shows the error.
func WriteLimited(w http.ResponseWriter, r *http.Request, kit layout.Kit, title string, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if rl, ok := errors.AsType[*RateLimitedError](err); ok {
		w.Header().Set("Retry-After", strconv.Itoa(rl.RetryAfterSeconds()))
		kit.Render(w, r, http.StatusTooManyRequests, layout.Error(kit.Layout(title), rl.Message()))
		return
	}
	limiterFailed(w, r, kit, err)
	kit.Render(w, r, http.StatusServiceUnavailable, layout.Error(kit.Layout(title), limiterDown))
}

func limiterFailed(w http.ResponseWriter, r *http.Request, kit layout.Kit, err error) {
	w.Header().Set("Retry-After", strconv.Itoa(int(limiterDownRetry/time.Second)))
	if kit.Unexpected != nil {
		_ = kit.Unexpected(r.Context(), "web: rate limit", err)
	}
}

// ResetEmailRule limits reset emails per address: 3 an hour.
func ResetEmailRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "reset-email", Rate: 3, Per: time.Hour, Burst: 3}
}

// ResetIPRule limits reset requests per client IP: 20 an hour.
func ResetIPRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "reset-ip", Rate: 20, Per: time.Hour, Burst: 20}
}

// ResetLinkIPRule limits opening reset links and setting the new password per client IP: 20 an hour.
func ResetLinkIPRule() ratelimit.Rule {
	return ratelimit.Rule{Name: "reset-link-ip", Rate: 20, Per: time.Hour, Burst: 20}
}
