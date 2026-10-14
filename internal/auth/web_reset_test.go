package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

type resetWeb struct {
	*resetFixture
	srv http.Handler
}

func newResetWeb(t *testing.T, limits Limits) *resetWeb {
	t.Helper()
	f := &resetWeb{resetFixture: newResetFixture(t)}
	sessions := testSessions(f.sessions)
	sessions.Cookie.Path = "/"
	f.srv = web.NewServer(web.ServerOpts{
		Session: middleware.Session(middleware.SessionConfig{Store: f.sessions, Secret: testSecret, Auth: f.fixture.svc}),
		Routes: []web.Registrar{
			NewWeb(layout.Kit{}, f.fixture.svc, sessions, fakes.NewPublicIDs()).WithLimits(limits).WithResets(f.svc),
		},
	})
	return f
}

func (f *resetWeb) do(req *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	req.Header.Set("X-Request-Id", "same-request")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

func form(path string, v url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func (f *resetWeb) open(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	require.Equal(t, http.StatusOK, f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}})).Code)
	raw := linkToken(t, f.mail.sent[len(f.mail.sent)-1])
	rec := f.do(httptest.NewRequest(http.MethodGet, "/reset-password/"+raw, nil))
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	ck := cookieNamed(rec, "oagent_reset")
	require.NotNil(t, ck)
	return ck, raw
}

func TestResetWeb_LoginLinksToForgotPassword(t *testing.T) {
	f := newResetWeb(t, Limits{})
	body := f.do(httptest.NewRequest(http.MethodGet, "/login", nil)).Body.String()
	assert.Contains(t, body, `href="/forgot-password">Forgot password?</a>`)
	assert.NotContains(t, newLoginFixture(t, "").do(httptest.NewRequest(http.MethodGet, "/login", nil)).Body.String(), "Forgot password?",
		"no link when password reset is off")

	body = f.do(httptest.NewRequest(http.MethodGet, "/login?reset=done", nil)).Body.String()
	assert.Contains(t, body, "Your password was changed. Sign in with the new one.")
	body = f.do(httptest.NewRequest(http.MethodGet, "/forgot-password", nil)).Body.String()
	assert.Contains(t, body, `action="/forgot-password"`)
}

func TestResetWeb_RequestLooksTheSameForKnownAndUnknownEmails(t *testing.T) {
	for _, mode := range []string{"off", "on"} {
		t.Run(mode, func(t *testing.T) {
			outcome := func(known bool) (*httptest.ResponseRecorder, *resetWeb) {
				limits := Limits{}
				if mode == "on" {
					limits, _ = memoryLimits(t)
				}
				f := newResetWeb(t, limits)
				if known {
					f.addUser(t, "ada@example.com", true)
				}
				var rec *httptest.ResponseRecorder
				for range ResetEmailRule().Burst + 1 {
					rec = f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}}))
				}
				return rec, f
			}
			k, kf := outcome(true)
			u, uf := outcome(false)
			assert.Equal(t, k.Code, u.Code)
			assert.Equal(t, k.Header(), u.Header())
			assert.Equal(t, k.Body.String(), u.Body.String())
			assert.Empty(t, uf.mail.sent, "no email for an unknown address")
			if mode == "off" {
				assert.Equal(t, http.StatusOK, k.Code)
				assert.Contains(t, k.Body.String(), "If an account exists for that email, we sent a link")
			} else {
				assert.Equal(t, http.StatusTooManyRequests, k.Code)
			}
			assert.Len(t, kf.mail.sent, 1, "a repeat within two minutes sends nothing new")
		})
	}
}

func TestResetWeb_RepeatWithinTwoMinutesLooksTheSame(t *testing.T) {
	f := newResetWeb(t, Limits{})
	f.addUser(t, "ada@example.com", true)
	first := f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}}))
	f.now = f.now.Add(time.Minute)
	repeat := f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}}))
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, first.Code, repeat.Code)
	assert.Equal(t, first.Header(), repeat.Header())
	assert.Equal(t, first.Body.String(), repeat.Body.String())
	assert.Len(t, f.mail.sent, 1)

	f.now = f.now.Add(time.Minute)
	later := f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}}))
	assert.Equal(t, first.Body.String(), later.Body.String())
	assert.Len(t, f.mail.sent, 2, "after two minutes a new link replaces the first")
	assert.Len(t, f.store.rows, 1)
}

func TestResetWeb_RequestLimitedPerIP(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newResetWeb(t, limits)
	for i := range ResetIPRule().Burst {
		email := "p" + string(rune('a'+i)) + "@example.com"
		require.Equal(t, http.StatusOK, f.do(form("/forgot-password", url.Values{"email": {email}})).Code)
	}
	rec := f.do(form("/forgot-password", url.Values{"email": {"z@example.com"}}))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestResetWeb_LinkSetsACookieAndLeavesTheURL(t *testing.T) {
	f := newResetWeb(t, Limits{})
	u := f.addUser(t, "ada@example.com", true)
	require.Equal(t, http.StatusOK, f.do(form("/forgot-password", url.Values{"email": {"ada@example.com"}})).Code)
	raw := linkToken(t, f.mail.sent[0])
	sent := len(f.mail.sent)
	rows := len(f.store.rows)

	rec := f.do(httptest.NewRequest(http.MethodGet, "/reset-password/"+raw, nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/reset-password", rec.Header().Get("Location"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	ck := cookieNamed(rec, "oagent_reset")
	require.NotNil(t, ck)
	assert.True(t, ck.HttpOnly)
	assert.True(t, ck.Secure)
	assert.Equal(t, "/reset-password", ck.Path)
	assert.InDelta(t, ResetTTL.Seconds(), ck.MaxAge, 2)
	assert.NotContains(t, ck.Value, raw, "the token never goes into the cookie")
	assert.NotContains(t, ck.Value, u.ID.String(), "nor a UUID")
	for _, r := range f.store.rows {
		assert.NotContains(t, ck.Value, r.ID.String())
		assert.Contains(t, ck.Value, r.PublicID)
		assert.Nil(t, r.UsedAt, "a GET writes nothing")
	}
	assert.Equal(t, sent, len(f.mail.sent))
	assert.Equal(t, rows, len(f.store.rows))

	page := f.do(httptest.NewRequest(http.MethodGet, "/reset-password", nil), ck)
	assert.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, `value="ada@example.com"`)
	assert.Contains(t, body, `name="name_hint" value="Ada"`)
	assert.Contains(t, body, `data-email-input="email" data-name-input="name_hint"`)
	assert.NotContains(t, body, u.ID.String())
	assert.NotContains(t, body, raw)
}

func TestResetWeb_DeadLinks(t *testing.T) {
	f := newResetWeb(t, Limits{})
	f.addUser(t, "ada@example.com", true)
	ck, raw := f.open(t)
	for name, path := range map[string]string{"unknown": "/reset-password/nope", "valid then expired": "/reset-password/" + raw} {
		t.Run(name, func(t *testing.T) {
			if name != "unknown" {
				f.now = f.now.Add(ResetTTL)
			}
			rec := f.do(httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusGone, rec.Code)
			c := cookieNamed(rec, "oagent_reset")
			assert.True(t, c == nil || c.MaxAge < 0, "no reset cookie")
			if name == "unknown" {
				assert.Contains(t, rec.Body.String(), "This reset link is not valid.")
			} else {
				assert.Contains(t, rec.Body.String(), "This reset link has expired.")
				page := f.do(httptest.NewRequest(http.MethodGet, "/reset-password", nil), ck)
				assert.Equal(t, http.StatusGone, page.Code)
			}
		})
	}
	rec := f.do(httptest.NewRequest(http.MethodGet, "/reset-password", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "This reset link is incomplete.")
	rec = f.do(form("/reset-password", url.Values{"password": {"a brand new passphrase"}}))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResetWeb_SetPassword(t *testing.T) {
	f := newResetWeb(t, Limits{})
	u := f.addUser(t, "ada@example.com", true)
	require.NoError(t, f.sessions.Save(t.Context(), "old", sessionOf(u), time.Now().Add(time.Hour)))
	ck, _ := f.open(t)

	weak := f.do(form("/reset-password", url.Values{"password": {"ada@example.com-1"}}), ck)
	assert.Equal(t, http.StatusUnprocessableEntity, weak.Code)
	assert.Contains(t, weak.Body.String(), "Password must not contain your email")
	assert.Contains(t, weak.Body.String(), `value="ada@example.com"`)

	for _, r := range f.store.rows {
		assert.Nil(t, r.UsedAt, "a weak password does not use up the link")
	}
	rec := f.do(form("/reset-password", url.Values{"password": {"a brand new passphrase"}}), ck)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/login?reset=done", rec.Header().Get("Location"))
	cleared := cookieNamed(rec, "oagent_reset")
	require.NotNil(t, cleared)
	assert.Negative(t, cleared.MaxAge)
	sid := cookieNamed(rec, session.CookieName)
	require.NotNil(t, sid)
	assert.Negative(t, sid.MaxAge, "the session cookie is cleared too")
	_, ok, err := f.sessions.Load(t.Context(), "old")
	require.NoError(t, err)
	assert.False(t, ok, "every session ended")
	assert.NoError(t, f.fixture.svc.verify(u.PasswordHash, "a brand new passphrase"))

	again := f.do(form("/reset-password", url.Values{"password": {"another new passphrase"}}), ck)
	assert.Equal(t, http.StatusGone, again.Code)
	assert.Contains(t, again.Body.String(), "This reset link was already used.")
	for _, m := range f.mail.sent {
		assert.NotContains(t, m.TextBody+m.HTMLBody, u.ID.String(), "no UUID in any email")
	}
}

func TestResetWeb_LinkLimitedPerIP(t *testing.T) {
	limits, _ := memoryLimits(t)
	f := newResetWeb(t, limits)
	for range ResetLinkIPRule().Burst {
		require.Equal(t, http.StatusGone, f.do(httptest.NewRequest(http.MethodGet, "/reset-password/nope", nil)).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, f.do(httptest.NewRequest(http.MethodGet, "/reset-password/nope", nil)).Code)
	assert.Equal(t, http.StatusTooManyRequests, f.do(form("/reset-password", url.Values{"password": {"x"}})).Code)
}
