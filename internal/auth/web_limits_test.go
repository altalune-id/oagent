package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

func wrongLogin(email, ip string) *http.Request {
	req := loginPost("", url.Values{"email": {email}, "password": {"wrong password here"}})
	req.RemoteAddr = ip + ":4711"
	return req
}

func TestLogin_LimitedPerEmailAcrossIPs(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newLimitedLoginFixture(t, "", limits)
	for i := range 5 {
		rec := f.do(wrongLogin("ada@example.com", "192.0.2."+strconv.Itoa(i+1)))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d", i+1)
	}
	calls := f.auth.loginCalls
	rec := f.do(wrongLogin("ada@example.com", "192.0.2.99"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "90", rec.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Body.String(), "Too many attempts. Try again in 2 minutes.")
	assert.Equal(t, calls, f.auth.loginCalls, "a limited attempt never checks the password")

	rec = f.do(wrongLogin("ADA@example.com ", "192.0.2.100"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "the email key ignores case and spaces")
}

func TestLogin_LimitedPerIPAcrossEmails(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newLimitedLoginFixture(t, "", limits)
	for i := range LoginIPRule().Burst {
		rec := f.do(wrongLogin("person"+strconv.Itoa(i)+"@example.com", "198.51.100.7"))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d", i+1)
	}
	rec := f.do(wrongLogin("someone-new@example.com", "198.51.100.7"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusUnauthorized, f.do(wrongLogin("someone-new@example.com", "198.51.100.8")).Code)
}

func TestLogin_SuccessBeforeTheLimitWorksAndResetsTheEmail(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newLimitedLoginFixture(t, "", limits)
	for range 4 {
		require.Equal(t, http.StatusUnauthorized, f.do(wrongLogin("ada@example.com", "192.0.2.1")).Code)
	}
	ok := loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}})
	require.Equal(t, http.StatusSeeOther, f.do(ok).Code)
	for i := range 5 {
		assert.Equal(t, http.StatusUnauthorized, f.do(wrongLogin("ada@example.com", "192.0.2.1")).Code, "after reset, attempt %d", i+1)
	}
}

func TestLogin_LimitedJSON(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newLimitedLoginFixture(t, "", limits)
	for range 5 {
		f.do(wrongLogin("ada@example.com", "192.0.2.1"))
	}
	req := wrongLogin("ada@example.com", "192.0.2.1")
	req.Header.Set("Accept", "application/json")
	rec := f.do(req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "90", rec.Header().Get("Retry-After"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, string(apperror.CodeAuthRateLimited), body["code"])
	assert.EqualValues(t, 90, body["retry_after"])
	assert.Equal(t, "Too many attempts. Try again in 2 minutes.", body["message"])
}

func TestLogin_BrokenLimiterFailsClosed(t *testing.T) {
	broken := &brokenLimiter{}
	f := newLimitedLoginFixture(t, "", Limits{Limiter: broken})
	rec := f.do(loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "Sign-in is temporarily unavailable")
	assert.Nil(t, cookieNamed(rec, session.CookieName), "no session when the limiter is down")
	assert.Zero(t, f.auth.loginCalls)
	assert.Equal(t, []string{"web: rate limit"}, f.reported, "reported once")

	req := loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"x"}})
	req.Header.Set("Accept", "application/json")
	rec = f.do(req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"code":"`+string(apperror.CodeUnexpected)+`","message":"Sign-in is temporarily unavailable. Try again in a moment."}`, rec.Body.String())
}

func TestLogin_OffByDefaultNeverLimits(t *testing.T) {
	for name, limits := range map[string]Limits{"zero value": {}, "disabled": {Limiter: ratelimit.Disabled()}} {
		t.Run(name, func(t *testing.T) {
			f := newLimitedLoginFixture(t, "", limits)
			for i := range 100 {
				rec := f.do(wrongLogin("ada@example.com", "192.0.2.1"))
				require.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d", i+1)
				require.Empty(t, rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestLogin_KnownAndUnknownEmailLookTheSame(t *testing.T) {
	type outcome struct {
		code   int
		header http.Header
		body   string
	}
	run := func(t *testing.T, known bool, limits Limits, accept string) (outcome, *fixture) {
		sf := newFixture(t, BasicCredentials{})
		if known {
			sf.addUser(t, "ada@example.com", true)
		}
		srv := web.NewServer(web.ServerOpts{
			Session: middleware.Session(middleware.SessionConfig{Store: sf.sessions, Secret: testSecret, Auth: sf.svc}),
			Routes:  []web.Registrar{NewWeb(layout.Kit{}, sf.svc, testSessions(sf.sessions), fakes.NewPublicIDs()).WithLimits(limits)},
		})
		var last *httptest.ResponseRecorder
		for range 6 {
			req := wrongLogin("ada@example.com", "192.0.2.1")
			req.Header.Set("X-Request-Id", "same-request")
			if accept != "" {
				req.Header.Set("Accept", accept)
			}
			last = httptest.NewRecorder()
			srv.ServeHTTP(last, req)
		}
		return outcome{last.Code, last.Header(), last.Body.String()}, sf
	}
	for _, mode := range []string{"off", "on"} {
		for _, accept := range []string{"", "application/json"} {
			t.Run(mode+" "+accept, func(t *testing.T) {
				limits := func() Limits {
					if mode == "on" {
						l, _ := memoryLimits(t)
						return l
					}
					return Limits{}
				}
				known, kf := run(t, true, limits(), accept)
				unknown, uf := run(t, false, limits(), accept)
				assert.Equal(t, known, unknown)
				if mode == "off" {
					assert.Equal(t, http.StatusUnauthorized, known.code)
					assert.Equal(t, 6, kf.verifies, "a known email checks its hash")
					assert.Equal(t, 6, uf.dummies, "an unknown email does the same hash work")
				} else {
					assert.Equal(t, http.StatusTooManyRequests, known.code)
					assert.Equal(t, 5, kf.verifies+kf.dummies)
					assert.Equal(t, 5, uf.verifies+uf.dummies)
				}
			})
		}
	}
}
