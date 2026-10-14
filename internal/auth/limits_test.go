package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/kernel/ratelimit/ratelimittest"
	"altalune.id/oagent/internal/web/layout"
)

type brokenLimiter struct{ calls int }

func (b *brokenLimiter) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	b.calls++
	return ratelimit.Result{}, errors.New("store down")
}

func (b *brokenLimiter) Reset(context.Context, string) error { return errors.New("store down") }

type recordingLimiter struct{ keys []string }

func (r *recordingLimiter) Allow(_ context.Context, key string, _ ratelimit.Rule) (ratelimit.Result, error) {
	r.keys = append(r.keys, key)
	return ratelimit.Result{Allowed: true}, nil
}

func (r *recordingLimiter) Reset(context.Context, string) error { return nil }

func memoryLimits(t *testing.T) (Limits, *ratelimittest.Clock) {
	t.Helper()
	clock := ratelimittest.NewClock()
	return Limits{Limiter: memory.New(10_000, memory.WithClock(clock.Now))}, clock
}

func TestLimits_ZeroValueLimitsNothing(t *testing.T) {
	var l Limits
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	for range 1000 {
		require.NoError(t, l.LimitIP(r, LoginIPRule()))
		require.NoError(t, l.LimitEmail(t.Context(), "a@x.io", LoginEmailRule()))
	}
	require.NoError(t, l.ForgetEmail(t.Context(), "a@x.io", LoginEmailRule()))
}

func TestLimits_KeysByClientIPAndEmailHash(t *testing.T) {
	rec := &recordingLimiter{}
	l := Limits{Limiter: rec, TrustedProxyHops: 2}
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.1:1"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9, 198.51.100.7")
	require.NoError(t, l.LimitIP(r, LoginIPRule()))
	require.NoError(t, l.LimitEmail(t.Context(), " Ada@Example.com", LoginEmailRule()))
	assert.Equal(t, []string{
		"login-ip:ip:203.0.113.9",
		ratelimit.Key(LoginEmailRule(), ratelimit.EmailSubject("ada@example.com")),
	}, rec.keys)
}

func TestLimits_OverTheLimit(t *testing.T) {
	l, _ := memoryLimits(t)
	rule := LoginEmailRule()
	for range rule.Burst {
		require.NoError(t, l.LimitEmail(t.Context(), "a@x.io", rule))
	}
	err := l.LimitEmail(t.Context(), "a@x.io", rule)
	var rl *RateLimitedError
	require.ErrorAs(t, err, &rl)
	assert.Equal(t, 90*time.Second, rl.RetryAfter)

	require.NoError(t, l.ForgetEmail(t.Context(), "a@x.io", rule))
	assert.NoError(t, l.LimitEmail(t.Context(), "a@x.io", rule))
}

func TestLimits_ABrokenLimiterIsAnError(t *testing.T) {
	l := Limits{Limiter: &brokenLimiter{}}
	err := l.LimitIP(httptest.NewRequest(http.MethodPost, "/login", nil), LoginIPRule())
	require.Error(t, err)
	assert.False(t, IsRateLimitedError(err))
	assert.Error(t, l.LimitEmail(t.Context(), "a@x.io", LoginEmailRule()))
	assert.Error(t, l.ForgetEmail(t.Context(), "a@x.io", LoginEmailRule()))
}

func TestRules(t *testing.T) {
	for _, r := range []ratelimit.Rule{LoginEmailRule(), LoginIPRule(), ResetEmailRule(), ResetIPRule(), ResetLinkIPRule()} {
		require.NoError(t, r.Validate(), r.Name)
	}
	assert.Equal(t, ratelimit.Rule{Name: "login-email", Rate: 10, Per: 15 * time.Minute, Burst: 5}, LoginEmailRule())
	assert.Equal(t, ratelimit.Rule{Name: "login-ip", Rate: 60, Per: 15 * time.Minute, Burst: 60}, LoginIPRule())
	assert.Equal(t, ratelimit.Rule{Name: "reset-email", Rate: 3, Per: time.Hour, Burst: 3}, ResetEmailRule())
	assert.Equal(t, ratelimit.Rule{Name: "reset-ip", Rate: 20, Per: time.Hour, Burst: 20}, ResetIPRule())
	assert.Equal(t, ratelimit.Rule{Name: "reset-link-ip", Rate: 20, Per: time.Hour, Burst: 20}, ResetLinkIPRule())
}

func TestWriteLimited(t *testing.T) {
	var reported []string
	kit := layout.Kit{Unexpected: func(_ context.Context, msg string, _ error, _ ...any) *apperror.AppError {
		reported = append(reported, msg)
		return nil
	}}

	rec := httptest.NewRecorder()
	WriteLimited(rec, httptest.NewRequest(http.MethodGet, "/x", nil), kit, "Invite", &RateLimitedError{RetryAfter: 61 * time.Second})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "61", rec.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Body.String(), "Too many attempts. Try again in 2 minutes.")
	assert.Empty(t, reported)

	rec = httptest.NewRecorder()
	WriteLimited(rec, httptest.NewRequest(http.MethodGet, "/x", nil), kit, "Invite", errors.New("store down"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "5", rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "Sign-in is temporarily unavailable")
	assert.NotContains(t, rec.Body.String(), "store down")
	assert.Equal(t, []string{"web: rate limit"}, reported)
}
