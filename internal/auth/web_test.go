package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

type loginFixture struct {
	reported []string
	srv      http.Handler
	auth     *fakeAuth
	ids      *fakes.PublicIDs
	store    *session.MemoryStore
	base     string
}

func newLoginFixture(t *testing.T, base string) *loginFixture {
	t.Helper()
	return newLimitedLoginFixture(t, base, Limits{})
}

func newLimitedLoginFixture(t *testing.T, base string, limits Limits) *loginFixture {
	t.Helper()
	f := &loginFixture{
		base:  base,
		ids:   fakes.NewPublicIDs(),
		store: session.NewMemoryStore(),
		auth: &fakeAuth{
			users: map[string]string{"ada@example.com": "correct horse battery"},
			principal: session.Principal{
				UserID: uuid.New(), Source: session.SourceLocal, Name: "Ada",
				ActiveOrgID: uuid.New(), ActiveProjectID: uuid.New(), Scopes: []string{"*"},
			},
		},
	}
	p := f.auth.principal
	f.ids.User(p.UserID)
	f.ids.Org(p.ActiveOrgID)
	f.ids.Project(p.ActiveProjectID, p.ActiveOrgID)
	sessions := testSessions(f.store)
	sessions.Cookie.Path = layout.Path(base, "")
	console := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("console:" + session.PrincipalFrom(r.Context()).Email))
	})
	f.srv = web.NewServer(web.ServerOpts{
		BasePath: base,
		Console:  console,
		Session:  middleware.Session(middleware.SessionConfig{Store: f.store, Secret: testSecret, Auth: f.auth}),
		Routes:   []web.Registrar{NewWeb(layout.Kit{BasePath: base, Unexpected: f.report}, f.auth, sessions, f.ids).WithLimits(limits)},
	})
	return f
}

func (f *loginFixture) report(_ context.Context, msg string, _ error, _ ...any) *apperror.AppError {
	f.reported = append(f.reported, msg)
	return nil
}

func (f *loginFixture) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

func loginPost(base string, form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, base+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func (f *loginFixture) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	rec := f.do(loginPost(f.base, url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	c := cookieNamed(rec, session.CookieName)
	require.NotNil(t, c)
	return c
}

func TestLogin_Form(t *testing.T) {
	f := newLoginFixture(t, "/app")
	rec := f.do(httptest.NewRequest(http.MethodGet, "/app/login?return_to=/app/console/x", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `name="password"`)
	assert.Contains(t, body, `value="/app/console/x"`)
	assert.Contains(t, body, `action="/app/login"`)
}

func TestLogin_FormLocksAGivenEmail(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(httptest.NewRequest(http.MethodGet, "/login?email=A%40X.io&return_to=/invite", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `name="email" value="a@x.io" placeholder="Email" autocomplete="username" required readonly`)
	assert.Contains(t, body, `value="/invite"`)
	assert.Contains(t, body, `href="/login?return_to=%2Finvite">Use another account</a>`)
	assert.Contains(t, body, `autocomplete="current-password" required autofocus`, "the password field takes focus instead")
}

func TestLogin_FormIgnoresABadEmail(t *testing.T) {
	f := newLoginFixture(t, "")
	for _, raw := range []string{"%3Cscript%3E", strings.Repeat("a", 250) + "%40x.io", ""} {
		rec := f.do(httptest.NewRequest(http.MethodGet, "/login?return_to=/invite&email="+raw, nil))
		body := rec.Body.String()
		assert.NotContains(t, body, "readonly", raw)
		assert.NotContains(t, body, "<script>", raw)
		assert.NotContains(t, body, "Use another account", raw)
	}
}

func TestLogin_FormWithoutEmailIsUnchanged(t *testing.T) {
	f := newLoginFixture(t, "")
	body := f.do(httptest.NewRequest(http.MethodGet, "/login?return_to=/invite", nil)).Body.String()
	assert.Contains(t, body, `name="email" value="" placeholder="Email" autocomplete="username" required autofocus`)
	assert.NotContains(t, body, "readonly")
}

func TestLogin_FormDropsUnsafeReturnTo(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(httptest.NewRequest(http.MethodGet, "/login?return_to=//evil.com", nil))
	assert.NotContains(t, rec.Body.String(), "evil.com")
}

func TestLogin_SuccessHTML(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		t.Run(base, func(t *testing.T) {
			f := newLoginFixture(t, base)
			form := url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}, "return_to": {base + "/console/o/acme"}}
			rec := f.do(loginPost(base, form))
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, base+"/console/o/acme", rec.Header().Get("Location"))
			c := cookieNamed(rec, session.CookieName)
			require.NotNil(t, c)
			assert.Equal(t, layout.Path(base, ""), c.Path)

			req := httptest.NewRequest(http.MethodGet, base+"/console/", nil)
			req.AddCookie(c)
			assert.Equal(t, "console:ada@example.com", f.do(req).Body.String(), "the console sees the principal")
		})
	}
}

func TestLogin_OpenRedirectFallsBackToConsole(t *testing.T) {
	f := newLoginFixture(t, "")
	for _, rt := range []string{"//evil.com", `/\evil.com`, "https://evil.com", "javascript:alert(1)"} {
		rec := f.do(loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}, "return_to": {rt}}))
		assert.Equal(t, "/console/", rec.Header().Get("Location"), rt)
	}
}

func TestLogin_FailureHTML(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"wrong password!"}}))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Invalid email or password")
	assert.Contains(t, body, `value="ada@example.com"`)
	assert.NotContains(t, body, "wrong password!")
	assert.Nil(t, cookieNamed(rec, session.CookieName))
}

func jsonLogin(base string, form url.Values) *http.Request {
	req := loginPost(base, form)
	req.Header.Set("Accept", "application/json")
	return req
}

func TestLogin_SuccessJSON(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(jsonLogin("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Session   string    `json:"session"`
		ExpiresAt time.Time `json:"expires_at"`
		Principal struct {
			Source          string   `json:"source"`
			UserID          string   `json:"user_id"`
			Email           string   `json:"email"`
			Name            string   `json:"name"`
			ActiveOrgID     string   `json:"active_org_id"`
			ActiveProjectID string   `json:"active_project_id"`
			Permissions     []string `json:"permissions"`
			Operator        bool     `json:"operator"`
		} `json:"principal"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	c := cookieNamed(rec, session.CookieName)
	require.NotNil(t, c, "JSON login still sets the cookie")
	assert.Equal(t, c.Value, body.Session)
	assert.WithinDuration(t, time.Now().Add(time.Hour), body.ExpiresAt, time.Minute)
	assert.Equal(t, "local", body.Principal.Source)
	p := f.auth.principal
	assert.Equal(t, f.ids.User(p.UserID), body.Principal.UserID)
	assert.Equal(t, "ada@example.com", body.Principal.Email)
	assert.Equal(t, f.ids.Org(p.ActiveOrgID), body.Principal.ActiveOrgID)
	assert.Equal(t, f.ids.Project(p.ActiveProjectID, p.ActiveOrgID), body.Principal.ActiveProjectID)
	assert.NotContains(t, rec.Body.String(), p.UserID.String())
	assert.NotContains(t, rec.Body.String(), p.ActiveOrgID.String())
	assert.NotContains(t, rec.Body.String(), p.ActiveProjectID.String())
	assert.Equal(t, []string{"*"}, body.Principal.Permissions)

	sid, err := session.Verify(testSecret, body.Session)
	require.NoError(t, err)
	_, ok, err := f.store.Load(context.Background(), sid)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLogin_JSONPublicIDLookupFailureIs500(t *testing.T) {
	f := newLoginFixture(t, "")
	f.ids.Err = errors.New("db down")
	rec := f.do(jsonLogin("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db down")
	assert.NotContains(t, rec.Body.String(), f.auth.principal.UserID.String())
	for _, c := range rec.Result().Cookies() {
		assert.NotEqual(t, session.CookieName, c.Name, "no session starts when the login view fails")
	}
}

func TestLogin_FailureJSON(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(jsonLogin("", url.Values{"email": {"nobody@example.com"}, "password": {"whatever-pass"}}))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"code":"AUT001","message":"Invalid email or password"}`, rec.Body.String())
}

func TestLogin_SignedInSkipsForm(t *testing.T) {
	f := newLoginFixture(t, "")
	c := f.signIn(t)
	req := httptest.NewRequest(http.MethodGet, "/login?return_to=/console/x", nil)
	req.AddCookie(c)
	rec := f.do(req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/console/x", rec.Header().Get("Location"))
}

func TestLogout(t *testing.T) {
	f := newLoginFixture(t, "/app")
	c := f.signIn(t)
	sid, err := session.Verify(testSecret, c.Value)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/app/logout", nil)
	req.AddCookie(c)
	rec := f.do(req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app/login", rec.Header().Get("Location"))
	assert.Equal(t, []string{sid}, f.auth.loggedOut)
	cleared := cookieNamed(rec, session.CookieName)
	require.NotNil(t, cleared)
	assert.Equal(t, -1, cleared.MaxAge)

	assert.Equal(t, http.StatusMethodNotAllowed, f.do(httptest.NewRequest(http.MethodGet, "/app/logout", nil)).Code)
}

func TestLogout_WithoutSession(t *testing.T) {
	f := newLoginFixture(t, "")
	rec := f.do(httptest.NewRequest(http.MethodPost, "/logout", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Empty(t, f.auth.loggedOut)
}

func TestLogin_UnexpectedErrorIs500(t *testing.T) {
	f := newLoginFixture(t, "")
	f.auth.loginErr = errors.New("db down")
	rec := f.do(loginPost("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db down")
}

func TestLogin_JSONWithoutScopesSendsEmptyPermissions(t *testing.T) {
	f := newLoginFixture(t, "")
	f.auth.principal.Scopes = nil
	rec := f.do(jsonLogin("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"permissions":[]`)
}

func TestLogin_SessionStartFailureIs500(t *testing.T) {
	signIn := &fakeAuth{users: map[string]string{"ada@example.com": "correct horse battery"}}
	sessions := testSessions(failingStore{MemoryStore: session.NewMemoryStore(), saveErr: errors.New("store down")})
	mux := http.NewServeMux()
	NewWeb(layout.Kit{}, signIn, sessions, fakes.NewPublicIDs()).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, jsonLogin("", url.Values{"email": {"ada@example.com"}, "password": {"correct horse battery"}}))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "store down")
	assert.Nil(t, cookieNamed(rec, session.CookieName))
}

func TestLogout_ServiceErrorIsReportedAndCookieCleared(t *testing.T) {
	store := session.NewMemoryStore()
	sessions := testSessions(store)
	signIn := &fakeAuth{logoutErr: errors.New("db down")}
	var reported []error
	kit := layout.Kit{Unexpected: func(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
		assert.Equal(t, "web: logout", msg)
		reported = append(reported, cause)
		return nil
	}}
	mux := http.NewServeMux()
	NewWeb(kit, signIn, sessions, fakes.NewPublicIDs()).Register(mux)

	login := httptest.NewRecorder()
	_, _, err := sessions.Start(context.Background(), login, session.Principal{UserID: uuid.New(), Source: session.SourceLocal})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(login.Result().Cookies()[0])

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
	assert.Len(t, signIn.loggedOut, 1)
	assert.Equal(t, []error{signIn.logoutErr}, reported)
	cleared := cookieNamed(rec, session.CookieName)
	require.NotNil(t, cleared)
	assert.Equal(t, -1, cleared.MaxAge)
}

type fakeAuth struct {
	users      map[string]string
	principal  session.Principal
	loggedOut  []string
	loginCalls int
	loginErr   error
	logoutErr  error
}

func (f *fakeAuth) LoginLocal(_ context.Context, c Credentials) (session.Principal, error) {
	f.loginCalls++
	if f.loginErr != nil {
		return session.Principal{}, f.loginErr
	}
	if pw, ok := f.users[c.Email]; !ok || pw != c.Password {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	p := f.principal
	p.Email = c.Email
	return p, nil
}

func (f *fakeAuth) Logout(_ context.Context, sid string) error {
	f.loggedOut = append(f.loggedOut, sid)
	return f.logoutErr
}

func (f *fakeAuth) Refresh(_ context.Context, p session.Principal) (session.Principal, error) {
	return p, nil
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
