package invite_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
)

type downLimiter struct{}

func (downLimiter) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("store down")
}

func (downLimiter) Reset(context.Context, string) error { return nil }

func memLimits() auth.Limits { return auth.Limits{Limiter: memory.New(10_000)} }

func linkPath(c *invite.Created) string {
	return strings.TrimPrefix(c.Link, "https://oagent.example.com")
}

func TestRules(t *testing.T) {
	assert.Equal(t, ratelimit.Rule{Name: "invite-landing-ip", Rate: 60, Per: time.Hour, Burst: 60}, invite.LandingRule())
	assert.Equal(t, ratelimit.Rule{Name: "invite-submit-ip", Rate: 20, Per: time.Hour, Burst: 20}, invite.SubmitRule())
}

func TestWeb_LandingLimitedPerIP(t *testing.T) {
	f := newLimitedWebFixture(t, false, memLimits())
	c := f.create("bob@x.io", member.RoleMember)
	ck := f.open(t, c)
	for i := range invite.LandingRule().Burst - 2 {
		require.Equal(t, http.StatusOK, f.get("/invite", ck).Code, "visit %d", i+1)
	}
	require.Equal(t, http.StatusSeeOther, f.get(linkPath(c)).Code)
	rec := f.get(linkPath(c))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Body.String(), "Too many attempts.")
	assert.Nil(t, cookieNamed(rec, "oagent_invite"), "a limited link sets nothing")
	assert.Equal(t, http.StatusTooManyRequests, f.get("/invite", ck).Code)
}

func TestWeb_JoinAndAcceptLimitedPerIP(t *testing.T) {
	f := newLimitedWebFixture(t, false, memLimits())
	c := f.create("bob@x.io", member.RoleMember)
	ck := f.open(t, c)
	for i := range invite.SubmitRule().Burst {
		path := "/invite/join"
		if i%2 == 1 {
			path = "/invite/accept"
		}
		rec := f.post(path, joinForm("", "short"), ck)
		require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "attempt %d", i+1)
	}
	for _, path := range []string{"/invite/join", "/invite/accept"} {
		rec := f.post(path, joinForm("Bob", "a sturdy passphrase"), ck)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, path)
		assert.NotEmpty(t, rec.Header().Get("Retry-After"), path)
	}
	_, err := f.users.ByEmail(f.ctx, "bob@x.io")
	assert.Error(t, err, "a limited join creates no account")
}

func TestWeb_BrokenLimiterFailsClosed(t *testing.T) {
	f := newLimitedWebFixture(t, false, auth.Limits{Limiter: downLimiter{}})
	c := f.create("bob@x.io", member.RoleMember)
	rec := f.get(linkPath(c))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "temporarily unavailable")
	assert.Nil(t, cookieNamed(rec, "oagent_invite"))
	assert.Equal(t, http.StatusServiceUnavailable, f.post("/invite/join", joinForm("Bob", "a sturdy passphrase")).Code)
}

func TestWeb_JoinLimitedTheSameForKnownAndUnknownEmails(t *testing.T) {
	outcome := func(t *testing.T, known bool) (int, http.Header, string) {
		f := newLimitedWebFixture(t, false, memLimits())
		if known {
			f.account(t, "bob@x.io")
		}
		ck := f.open(t, f.create("bob@x.io", member.RoleMember))
		var code int
		var header http.Header
		var body string
		for range invite.SubmitRule().Burst + 1 {
			req := httptest.NewRequest(http.MethodPost, "/invite/join", strings.NewReader(joinForm("Bob", "short").Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Request-Id", "same-request")
			rec := f.do(req, ck)
			code, header, body = rec.Code, rec.Header(), rec.Body.String()
		}
		return code, header, body
	}
	kc, kh, kb := outcome(t, true)
	uc, uh, ub := outcome(t, false)
	assert.Equal(t, http.StatusTooManyRequests, kc)
	assert.Equal(t, kc, uc)
	assert.Equal(t, kh, uh)
	assert.Equal(t, kb, ub)
}

func TestWeb_OffNeverLimits(t *testing.T) {
	f := newLimitedWebFixture(t, false, auth.Limits{Limiter: ratelimit.Disabled()})
	c := f.create("bob@x.io", member.RoleMember)
	for range 100 {
		require.Equal(t, http.StatusSeeOther, f.get(linkPath(c)).Code)
	}
}
