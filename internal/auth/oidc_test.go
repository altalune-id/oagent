package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/altalune-id/othra/oidcrp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

type fakeRP struct {
	started  oidcrp.AuthRequest
	token    *oidcrp.IDToken
	err      error
	endHint  string
	endErr   error
	noEnd    bool
	startErr error
}

func (f *fakeRP) AuthCodeURL(_ context.Context, req oidcrp.AuthRequest) (string, error) {
	f.started = req
	return "http://idp.test/authorize?state=" + req.State, f.startErr
}

func (f *fakeRP) Exchange(_ context.Context, cb oidcrp.Callback, req oidcrp.AuthRequest) (*oidcrp.IDToken, error) {
	switch {
	case f.err != nil:
		return nil, f.err
	case cb.State != req.State:
		return nil, &oidcrp.Error{Reason: oidcrp.ReasonState, Err: errors.New("state")}
	}
	return f.token, nil
}

func (f *fakeRP) EndSessionURL(_ context.Context, hint, postLogout, _ string) (string, error) {
	f.endHint = hint
	if f.noEnd || f.endErr != nil {
		return "", f.endErr
	}
	return "http://idp.test/end-session?post_logout_redirect_uri=" + url.QueryEscape(postLogout), nil
}

type oidcWebFixture struct {
	*loginFixture
	rp  *fakeRP
	web *Web
}

func newOIDCWebFixture(t *testing.T, passwordOff bool) *oidcWebFixture {
	t.Helper()
	f := &oidcWebFixture{loginFixture: newLoginFixture(t, ""), rp: &fakeRP{token: &oidcrp.IDToken{
		Subject: "sub-1", Email: "ada@example.com", EmailVerified: true, Name: "Ada", Raw: "the.id.token",
	}}}
	sessions := testSessions(f.store)
	sessions.Cookie.Path = "/"
	f.web = NewWeb(layout.Kit{Unexpected: f.report}, f.auth, sessions, f.ids).WithOIDC(OIDCSignIn{
		Provider: f.rp, ButtonLabel: "Continue with Acme ID", SignedOut: "http://oagent.test/login",
	})
	if passwordOff {
		f.web.WithoutPasswords()
	}
	f.srv = web.NewServer(web.ServerOpts{
		Console: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("console")) }),
		Session: middleware.Session(middleware.SessionConfig{Store: f.store, Secret: testSecret, Auth: f.auth}),
		Routes:  []web.Registrar{f.web},
	})
	return f
}

func (f *oidcWebFixture) start(t *testing.T, query string) (*http.Cookie, string) {
	t.Helper()
	rec := f.do(httptest.NewRequest(http.MethodGet, "/login/oidc?"+query, nil))
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	c := cookieNamed(rec, signInCookieName)
	require.NotNil(t, c)
	return c, f.rp.started.State
}

func (f *oidcWebFixture) callback(query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?"+query, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return f.do(req)
}

func (f *oidcWebFixture) signedInByOIDC(t *testing.T) *http.Cookie {
	t.Helper()
	c, state := f.start(t, "")
	sid := cookieNamed(f.callback("code=c1&state="+state, c), session.CookieName)
	require.NotNil(t, sid)
	return sid
}

func (f *oidcWebFixture) logout(sid *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(sid)
	return f.do(req)
}

// SECURITY: RFC 9700 §2.1.1, each start mints its own state, nonce and verifier, kept only in a signed HttpOnly cookie on the callback.
func TestOIDCSignIn_Start(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	rec := f.do(httptest.NewRequest(http.MethodGet, "/login/oidc?return_to=/console/x&prompt=select_account&login_hint=Ada%40Example.com", nil))
	require.Equal(t, http.StatusFound, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "http://idp.test/authorize?state="))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	c := cookieNamed(rec, signInCookieName)
	require.NotNil(t, c)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, "/oauth/callback", c.Path)
	assert.NotContains(t, c.Value, f.rp.started.CodeVerifier, "the cookie is encoded, never the bare verifier")
	assert.Equal(t, "select_account", f.rp.started.Prompt)
	assert.Equal(t, "ada@example.com", f.rp.started.LoginHint)

	_ = f.do(httptest.NewRequest(http.MethodGet, "/login/oidc?prompt=none", nil))
	assert.Empty(t, f.rp.started.Prompt, "only select_account and login pass through")

	f.rp.startErr = errors.New("discovery down")
	rec = f.do(httptest.NewRequest(http.MethodGet, "/login/oidc", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestOIDCSignIn_Callback(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	f.auth.principal.Source, f.auth.principal.IDToken = session.SourceOIDC, "the.id.token"
	c, state := f.start(t, "return_to=/console/x&invite=inv-1")

	rec := f.callback("code=c1&state="+state, c)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/x", rec.Header().Get("Location"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	require.NotNil(t, cookieNamed(rec, session.CookieName), "signed in")
	require.Len(t, f.auth.oidc, 1)
	assert.Equal(t, OIDCLogin{Subject: "sub-1", Email: "ada@example.com", EmailVerified: true, Name: "Ada", IDToken: "the.id.token", Invite: "inv-1"}, f.auth.oidc[0])
	cleared := cookieNamed(rec, signInCookieName)
	require.NotNil(t, cleared)
	assert.Negative(t, cleared.MaxAge, "the state is single use")
}

func TestOIDCSignIn_CallbackRefusals(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	rec := f.callback("code=c1&state=x")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "no state cookie")
	assert.Contains(t, rec.Body.String(), "This sign-in expired")
	assert.Nil(t, cookieNamed(rec, session.CookieName))

	bad := &http.Cookie{Name: signInCookieName, Value: "tampered"}
	rec = f.callback("code=c1&state=x", bad)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a tampered state cookie")

	c, _ := f.start(t, "")
	rec = f.callback("code=c1&state=forged", c)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a forged state")
	assert.Empty(t, f.auth.oidc)

	c, state := f.start(t, "")
	f.rp.err = &oidcrp.Error{Reason: oidcrp.ReasonCancelled, Err: errors.New("access_denied")}
	rec = f.callback("error=access_denied&state="+state, c)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"), "cancelled at the provider: back to sign in, quietly")

	for _, code := range []string{"invalid_grant", "invalid_request"} {
		c, state = f.start(t, "")
		f.rp.err = &oidcrp.Error{Reason: oidcrp.ReasonTokenExchange, OAuthError: code, Err: errors.New("status 400")}
		rec = f.callback("code=c1&state="+state, c)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "a replayed or unknown code is %s: start again", code)
		assert.Contains(t, rec.Body.String(), "This sign-in expired")
	}
	for _, reason := range []oidcrp.Reason{oidcrp.ReasonIssuer, oidcrp.ReasonNonce, oidcrp.ReasonIDToken, oidcrp.ReasonError} {
		c, state = f.start(t, "")
		f.rp.err = &oidcrp.Error{Reason: reason, Err: errors.New("upstream detail")}
		rec = f.callback("code=c1&state="+state, c)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "%s: start again", reason)
		assert.NotContains(t, rec.Body.String(), "upstream detail")
	}
	assert.Empty(t, f.reported, "a refused code is no server failure")

	for _, code := range []string{"", "invalid_client", "other"} {
		c, state = f.start(t, "")
		f.rp.err = &oidcrp.Error{Reason: oidcrp.ReasonTokenExchange, OAuthError: code, Err: errors.New("token endpoint down")}
		rec = f.callback("code=c1&state="+state, c)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "transport, 5xx or our own client: %q", code)
	}

	f.rp.err = nil
	f.auth.oidcErr = &NoConsoleAccountError{Reason: NoConsoleNotInvited}
	c, state = f.start(t, "return_to=/console/x")
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "/login/oidc?prompt=select_account")
	assert.Contains(t, rec.Body.String(), "You need an invite")
	assert.Nil(t, cookieNamed(rec, session.CookieName))

	f.auth.oidcErr = &IdentityConflictError{}
	c, state = f.start(t, "")
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "or with your password")

	f.auth.oidcErr = errors.New("db down")
	c, state = f.start(t, "")
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db down")

	off := newOIDCWebFixture(t, true)
	off.auth.oidcErr = &IdentityConflictError{}
	c, state = off.start(t, "")
	rec = off.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotContains(t, rec.Body.String(), "password", "no password to fall back on when passwords are off")
}

// NOTE: RP-Initiated Logout §2: a session from the provider ends there too, with its ID token as the hint.
func TestOIDCSignIn_Logout(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	f.auth.principal.Source, f.auth.principal.IDToken = session.SourceOIDC, "the.id.token"
	rec := f.logout(f.signedInByOIDC(t))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "http://idp.test/end-session?"))
	assert.Contains(t, rec.Header().Get("Location"), url.QueryEscape("http://oagent.test/login"))
	assert.Equal(t, "the.id.token", f.rp.endHint)
	assert.Len(t, f.auth.loggedOut, 1, "the local session ends first")

	f.rp.noEnd = true
	rec = f.logout(f.signedInByOIDC(t))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"), "no end_session_endpoint: back to sign-in, never an empty redirect")
	assert.Empty(t, f.reported, "a provider with no end-session endpoint is not a failure")

	f.rp.noEnd, f.rp.endErr = false, errors.New("discovery down")
	rec = f.logout(f.signedInByOIDC(t))
	assert.Equal(t, "/login", rec.Header().Get("Location"), "the local session still ends")
	assert.Equal(t, []string{"web: end the provider session"}, f.reported)

	f.rp.endErr, f.reported = nil, nil
	f.auth.principal.Source = session.SourceLocal
	rec = f.logout(f.signIn(t))
	assert.Equal(t, "/login", rec.Header().Get("Location"), "a password session never visits the provider")
}

func TestOIDCSignIn_LoginPage(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	rec := f.do(httptest.NewRequest(http.MethodGet, "/login?return_to=/console/x", nil))
	assert.Contains(t, rec.Body.String(), `/login/oidc?return_to=%2Fconsole%2Fx`)
	assert.Contains(t, rec.Body.String(), "Continue with Acme ID")
	assert.Contains(t, rec.Body.String(), `name="password"`)
	rec = f.do(httptest.NewRequest(http.MethodGet, "/login?email=Ada%40Example.com", nil))
	assert.Contains(t, rec.Body.String(), `/login/oidc?login_hint=ada%40example.com`)

	off := newOIDCWebFixture(t, true)
	rec = off.do(httptest.NewRequest(http.MethodGet, "/login", nil))
	assert.NotContains(t, rec.Body.String(), `name="password"`)
	rec = off.do(loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	assert.NotEqual(t, http.StatusSeeOther, rec.Code, "no password sign-in when passwords are off")
	assert.Nil(t, cookieNamed(rec, session.CookieName))
}

// SECURITY: the state's own issue time bounds it to ten minutes, whatever the browser does with the cookie's Max-Age.
func TestOIDCSignIn_StateExpires(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	now := time.Now()
	f.web.signInCookie.Now = func() time.Time { return now }
	c, state := f.start(t, "")

	now = now.Add(signInTTL + time.Second)
	rec := f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "This sign-in expired")
	assert.Empty(t, f.auth.oidc)

	now = now.Add(-2 * signInTTL)
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "issued in the future")

	now = now.Add(signInTTL)
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, http.StatusSeeOther, rec.Code, "within ten minutes: %s", rec.Body.String())
}

// The callback and the start share /login's IP limit; past it the exchange never runs.
func TestOIDCSignIn_LimitedPerIP(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	limits, _ := memoryLimits(t)
	f.web.WithLimits(limits)
	from := func(path, ip string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = ip + ":4711"
		return req
	}
	for i := range LoginIPRule().Burst {
		require.Equal(t, http.StatusBadRequest, f.do(from("/oauth/callback?code=c&state=s", "198.51.100.7")).Code, "attempt %d", i+1)
	}
	f.rp.err = errors.New("must not be called")
	rec := f.do(from("/oauth/callback?code=c&state=s", "198.51.100.7"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "Too many attempts")
	assert.Equal(t, http.StatusTooManyRequests, f.do(from("/login/oidc", "198.51.100.7")).Code, "the start shares the bucket")
	assert.Equal(t, http.StatusBadRequest, f.do(from("/oauth/callback?code=c&state=s", "198.51.100.8")).Code, "another IP goes on")
}

// A sign-in from an invite for another email lands on the invite, which shows Mismatch; one for this email goes on.
func TestOIDCSignIn_InviteForAnotherEmail(t *testing.T) {
	f := newOIDCWebFixture(t, false)
	f.auth.otherInvite = "inv-dan"
	c, state := f.start(t, "return_to=/console/x&invite=inv-dan")
	rec := f.callback("code=c1&state="+state, c)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/invite", rec.Header().Get("Location"))
	assert.NotNil(t, cookieNamed(rec, session.CookieName), "signed in, so the landing shows who is signed in")

	c, state = f.start(t, "return_to=/console/x&invite=inv-ada")
	rec = f.callback("code=c1&state="+state, c)
	assert.Equal(t, "/console/x", rec.Header().Get("Location"))
}
