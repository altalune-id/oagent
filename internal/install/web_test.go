package install

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/password"
)

const testToken = "setup-token-0123456789abcdefghij"

type installFixture struct {
	srv       http.Handler
	required  *atomic.Bool
	installer *fakeInstaller
	auth      *fakeAuth
	store     *session.MemoryStore
}

func newInstallFixture(t *testing.T, base string) *installFixture {
	t.Helper()
	return newInstallFixtureTerms(t, base, false)
}

func newInstallFixtureTerms(t *testing.T, base string, requireTerms bool) *installFixture {
	t.Helper()
	token, err := NewToken(testToken)
	require.NoError(t, err)
	f := &installFixture{
		required:  &atomic.Bool{},
		installer: &fakeInstaller{},
		auth:      &fakeAuth{users: map[string]string{}, principal: session.Principal{UserID: uuid.New(), Source: session.SourceLocal}},
		store:     session.NewMemoryStore(),
	}
	f.required.Store(true)
	sessions := auth.CookieSessions{
		Store:  f.store,
		Secret: testSecret,
		Cookie: session.CookieOptions{Path: layout.Path(base, ""), Secure: true},
		TTL:    time.Hour,
	}
	f.srv = web.NewServer(web.ServerOpts{
		BasePath: base,
		Gate:     InstallGate(base, f.required),
		Routes: []web.Registrar{NewWeb(WebOpts{
			Kit:       layout.Kit{BasePath: base},
			Installer: f.installer,
			SignIn:    f.auth,
			Sessions:  sessions,
			Token:     token,
			Required:  f.required,
			Secret:    testSecret,
			Secure:    true,
			Consent:   layout.ConsentData{Required: requireTerms},
		})},
	})
	return f
}

func (f *installFixture) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (f *installFixture) setupCookie(t *testing.T, base string) *http.Cookie {
	t.Helper()
	rec := f.do(t, httptest.NewRequest(http.MethodGet, base+"/install?token="+testToken, nil))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	c := cookieNamed(rec, setupCookieName)
	require.NotNil(t, c)
	return c
}

func installPost(base string, form url.Values, cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodPost, base+"/install", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

func goodForm() url.Values {
	return url.Values{"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"correct horse battery"}, "org_name": {"Acme"}}
}

func TestInstall_TokenGate(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		f := newInstallFixture(t, base)

		for name, path := range map[string]string{
			"no token, no cookie": base + "/install",
			"wrong token":         base + "/install?token=nope",
		} {
			t.Run(base+" "+name, func(t *testing.T) {
				rec := f.do(t, httptest.NewRequest(http.MethodGet, path, nil))
				assert.Equal(t, http.StatusForbidden, rec.Code)
				assert.Contains(t, rec.Body.String(), "Setup is locked")
				assert.Nil(t, cookieNamed(rec, setupCookieName))
			})
		}

		t.Run(base+" right token strips the URL and sets the setup cookie", func(t *testing.T) {
			rec := f.do(t, httptest.NewRequest(http.MethodGet, base+"/install?token="+testToken, nil))
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, base+"/install", rec.Header().Get("Location"))
			c := cookieNamed(rec, setupCookieName)
			require.NotNil(t, c)
			assert.NotContains(t, c.Value, testToken)
			assert.True(t, c.HttpOnly)
			assert.Equal(t, layout.Path(base, ""), c.Path)
			assert.Equal(t, 1800, c.MaxAge)

			req := httptest.NewRequest(http.MethodGet, base+"/install", nil)
			req.AddCookie(c)
			form := f.do(t, req)
			assert.Equal(t, http.StatusOK, form.Code)
			assert.Contains(t, form.Body.String(), `name="password"`)
			assert.NotContains(t, form.Body.String(), testToken)
		})
	}
}

func TestInstall_SetupCookieFromAnotherTokenIsRejected(t *testing.T) {
	f := newInstallFixture(t, "")
	other, err := NewToken("another-token-0123456789abcdefgh")
	require.NoError(t, err)
	forged := &http.Cookie{Name: setupCookieName, Value: setupValue(testSecret, other, f.now())}

	req := httptest.NewRequest(http.MethodGet, "/install", nil)
	req.AddCookie(forged)
	assert.Equal(t, http.StatusForbidden, f.do(t, req).Code)
}

func (f *installFixture) now() time.Time { return time.Now() }

func TestInstall_PostWithoutSetupCookie(t *testing.T) {
	f := newInstallFixture(t, "")
	rec := f.do(t, installPost("", goodForm()))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, f.installer.calls)
}

func TestInstall_Success(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		t.Run(base, func(t *testing.T) {
			f := newInstallFixture(t, base)
			f.auth.users["ada@example.com"] = "correct horse battery"
			setup := f.setupCookie(t, base)

			rec := f.do(t, installPost(base, goodForm(), setup))
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, base+"/console/", rec.Header().Get("Location"))
			require.Len(t, f.installer.calls, 1)
			assert.Equal(t, Request{Name: "Ada", Email: "ada@example.com", Password: "correct horse battery", OrgName: "Acme"}, f.installer.calls[0])

			sid := cookieNamed(rec, session.CookieName)
			require.NotNil(t, sid)
			assert.True(t, sid.HttpOnly)
			cleared := cookieNamed(rec, setupCookieName)
			require.NotNil(t, cleared)
			assert.Equal(t, -1, cleared.MaxAge)
			assert.False(t, f.required.Load(), "the gate opens")

			after := httptest.NewRequest(http.MethodGet, base+"/install", nil)
			after.AddCookie(setup)
			again := f.do(t, after)
			assert.Equal(t, http.StatusSeeOther, again.Code)
			assert.Equal(t, base+"/login", again.Header().Get("Location"))

			second := f.do(t, installPost(base, goodForm(), setup))
			assert.Equal(t, http.StatusSeeOther, second.Code)
			assert.Equal(t, base+"/login", second.Header().Get("Location"))
			assert.Len(t, f.installer.calls, 1, "no second install")
		})
	}
}

func TestInstall_AlreadyInstalledGoesToLogin(t *testing.T) {
	f := newInstallFixture(t, "")
	setup := f.setupCookie(t, "")
	f.installer.err = &AlreadyInstalledError{}

	rec := f.do(t, installPost("", goodForm(), setup))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
	assert.False(t, f.required.Load())
	assert.Nil(t, cookieNamed(rec, session.CookieName))
}

func TestInstall_ValidationErrorRerendersForm(t *testing.T) {
	f := newInstallFixture(t, "")
	setup := f.setupCookie(t, "")
	f.installer.err = &user.InvalidEmailError{}

	form := goodForm()
	form.Set("email", "nope")
	rec := f.do(t, installPost("", form, setup))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `value="Ada"`)
	assert.Contains(t, body, `value="nope"`)
	assert.Contains(t, body, `value="Acme"`)
	assert.NotContains(t, body, "correct horse battery")
	assert.Contains(t, body, "alert-error")
	assert.True(t, f.required.Load())
}

func TestInstall_PageHasThePasswordField(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		f := newInstallFixture(t, base)
		req := httptest.NewRequest(http.MethodGet, base+"/install", nil)
		req.AddCookie(f.setupCookie(t, base))
		rec := f.do(t, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, `data-password-field data-email-input="email" data-name-input="name"`)
		assert.Contains(t, body, `minlength="12"`)
		assert.Contains(t, body, `data-rule="length"`)
		assert.Contains(t, body, `<script type="module" src="`+base+`/static/password.js"></script>`)
	}
}

func TestInstall_WeakPasswordRerendersForm(t *testing.T) {
	for name, tc := range map[string]struct{ plain, message string }{
		"too short":     {"tea-for-two", "Password must be 12 to 256 characters"},
		"has the email": {"i-am-ada.lovelace", "Password must not contain your email"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newInstallFixture(t, "")
			setup := f.setupCookie(t, "")
			form := goodForm()
			form.Set("email", "ada.lovelace@example.com")
			form.Set("password", tc.plain)
			f.installer.err = password.Check(tc.plain, password.Hints{Email: form.Get("email"), Name: form.Get("name")})
			require.Error(t, f.installer.err)

			rec := f.do(t, installPost("", form, setup))
			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, tc.message)
			assert.NotContains(t, body, tc.plain)
			assert.Contains(t, body, `data-rule="length"`, "the rules show without JavaScript")
			assert.True(t, f.required.Load())
		})
	}
}

func TestInstall_CrossSitePostIsRejected(t *testing.T) {
	f := newInstallFixture(t, "")
	setup := f.setupCookie(t, "")
	req := installPost("", goodForm(), setup)
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	rec := f.do(t, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, f.installer.calls)
}

func TestInstall_RoutesAbsentWithoutInstaller(t *testing.T) {
	srv := web.NewServer(web.ServerOpts{Routes: []web.Registrar{NewWeb(WebOpts{})}})
	assert.Equal(t, http.StatusNotFound, get(t, srv, "/install").Code)
}

var testSecret = []byte("0123456789abcdef0123456789abcdef")

type fakeInstaller struct {
	mu    sync.Mutex
	calls []Request
	err   error
}

func (f *fakeInstaller) Install(_ context.Context, r Request) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	if f.err != nil {
		return nil, f.err
	}
	return &user.User{ID: uuid.New(), Email: r.Email, Name: r.Name}, nil
}

type fakeAuth struct {
	users     map[string]string
	principal session.Principal
}

func (f *fakeAuth) LoginLocal(_ context.Context, c auth.Credentials) (session.Principal, error) {
	if pw, ok := f.users[c.Email]; !ok || pw != c.Password {
		return session.Principal{}, &auth.InvalidCredentialsError{}
	}
	p := f.principal
	p.Email = c.Email
	return p, nil
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

type failingSessions struct{ err error }

func (f failingSessions) Start(context.Context, http.ResponseWriter, session.Principal) (string, time.Time, error) {
	return "", time.Time{}, f.err
}

type failingFixture struct {
	web      *Web
	required *atomic.Bool
	reported []string
}

func newFailingFixture(t *testing.T, installer Installer, signIn SignIn, sessions SessionStarter) *failingFixture {
	t.Helper()
	token, err := NewToken(testToken)
	require.NoError(t, err)
	f := &failingFixture{required: &atomic.Bool{}}
	f.required.Store(true)
	f.web = NewWeb(WebOpts{
		Kit: layout.Kit{Unexpected: func(_ context.Context, msg string, _ error, _ ...any) *apperror.AppError {
			f.reported = append(f.reported, msg)
			return nil
		}},
		Installer: installer,
		SignIn:    signIn,
		Sessions:  sessions,
		Token:     token,
		Required:  f.required,
		Secret:    testSecret,
	})
	return f
}

func (f *failingFixture) post(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	setup := &http.Cookie{Name: setupCookieName, Value: setupValue(testSecret, f.web.o.Token, time.Now())}
	rec := httptest.NewRecorder()
	f.web.postInstall(rec, installPost("", goodForm(), setup))
	return rec
}

func TestInstall_UnexpectedInstallErrorsFail(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"plain error", errors.New("db down")},
		{"app error of another kind", apperror.New(apperror.CodeUnexpected, "db down")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newFailingFixture(t, &fakeInstaller{err: tt.err}, &fakeAuth{}, failingSessions{})

			rec := f.post(t)

			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.Equal(t, []string{"web: install"}, f.reported)
			assert.NotContains(t, rec.Body.String(), "db down")
			assert.True(t, f.required.Load(), "the gate stays closed")
		})
	}
}

func TestInstall_AlreadyExistsRerendersForm(t *testing.T) {
	f := newFailingFixture(t, &fakeInstaller{err: &user.AlreadyExistsError{Email: "ada@example.com"}}, &fakeAuth{}, failingSessions{})

	rec := f.post(t)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "alert-error")
	assert.Empty(t, f.reported)
}

func TestInstall_SignInAfterInstallFails(t *testing.T) {
	f := newFailingFixture(t, &fakeInstaller{}, &fakeAuth{users: map[string]string{}}, failingSessions{})

	rec := f.post(t)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, []string{"web: sign in after install"}, f.reported)
	assert.False(t, f.required.Load(), "the install itself succeeded")
}

func TestInstall_StartSessionAfterInstallFails(t *testing.T) {
	signIn := &fakeAuth{users: map[string]string{"ada@example.com": "correct horse battery"}}
	f := newFailingFixture(t, &fakeInstaller{}, signIn, failingSessions{err: errors.New("store down")})

	rec := f.post(t)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, []string{"web: start session after install"}, f.reported)
	assert.Nil(t, cookieNamed(rec, session.CookieName))
}

func TestValidSetup(t *testing.T) {
	token, err := NewToken(testToken)
	require.NoError(t, err)
	now := time.Now()
	sign := func(v string) string { return session.Sign(testSecret, v) }
	cases := []struct {
		name  string
		token Token
		raw   string
		want  bool
	}{
		{"valid", token, setupValue(testSecret, token, now), true},
		{"bad signature", token, setupValue([]byte("another-secret-0123456789abcdefg"), token, now), false},
		{"not signed", token, "garbage", false},
		{"no separator", token, sign("12345"), false},
		{"empty token", Token{}, setupValue(testSecret, Token{}, now), false},
		{"non-numeric expiry", token, sign("soon." + tokenTag(token)), false},
		{"expired", token, sign(strconv.FormatInt(now.Add(-time.Minute).Unix(), 10) + "." + tokenTag(token)), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validSetup(testSecret, tt.token, tt.raw, now))
		})
	}
}

func TestInstall_Consent(t *testing.T) {
	t.Run("required", func(t *testing.T) {
		f := newInstallFixtureTerms(t, "", true)
		c := f.setupCookie(t, "")
		req := httptest.NewRequest(http.MethodGet, "/install", nil)
		req.AddCookie(c)
		page := f.do(t, req)
		assert.Contains(t, page.Body.String(), `name="accept_terms" value="1" required`)
		assert.Contains(t, page.Body.String(), `href="/terms"`)

		rec := f.do(t, installPost("", goodForm(), c))
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
		assert.Contains(t, rec.Body.String(), "Please accept the terms to continue.")
		assert.NotContains(t, rec.Body.String(), "correct horse battery", "the password is not echoed")
		assert.Empty(t, f.installer.calls, "no user is created")

		form := goodForm()
		form.Set("accept_terms", "1")
		f.auth.users["ada@example.com"] = "correct horse battery"
		rec = f.do(t, installPost("", form, c))
		require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
		require.Len(t, f.installer.calls, 1)
		assert.True(t, f.installer.calls[0].AcceptTerms)
	})
	t.Run("not required", func(t *testing.T) {
		f := newInstallFixture(t, "")
		c := f.setupCookie(t, "")
		req := httptest.NewRequest(http.MethodGet, "/install", nil)
		req.AddCookie(c)
		page := f.do(t, req).Body.String()
		assert.NotContains(t, page, "accept_terms")
		assert.Contains(t, page, "By continuing, you agree to the")

		form := goodForm()
		form.Set("accept_terms", "1")
		f.auth.users["ada@example.com"] = "correct horse battery"
		rec := f.do(t, installPost("", form, c))
		require.Equal(t, http.StatusSeeOther, rec.Code)
		require.Len(t, f.installer.calls, 1)
		assert.False(t, f.installer.calls[0].AcceptTerms, "acceptance is recorded only when it is asked for")
	})
}
