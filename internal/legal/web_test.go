package legal

import (
	"context"
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
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

var termsUpdated = NewService(nil).TermsUpdatedAt()

type fakeUsers struct{ accepted []uuid.UUID }

func (f *fakeUsers) AcceptTerms(_ context.Context, id uuid.UUID) (*user.User, error) {
	f.accepted = append(f.accepted, id)
	now := time.Now().UTC()
	return &user.User{ID: id, TermsAcceptedAt: &now}, nil
}

type welcomeFixture struct {
	srv   http.Handler
	store *session.MemoryStore
	users *fakeUsers
	base  string
}

func newWelcomeFixture(t *testing.T, base string, on bool) *welcomeFixture {
	t.Helper()
	f := &welcomeFixture{base: base, store: session.NewMemoryStore(), users: &fakeUsers{}}
	sessions := auth.CookieSessions{
		Store:  f.store,
		Secret: testSecret,
		Cookie: session.CookieOptions{Path: layout.Path(base, ""), Secure: true},
		TTL:    time.Hour,
	}
	console := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("console")) })
	f.srv = web.NewServer(web.ServerOpts{
		BasePath:    base,
		Console:     console,
		Session:     middleware.Session(middleware.SessionConfig{Store: f.store, Secret: testSecret, Auth: noRefresh{}}),
		SessionGate: WelcomeGate(base, on, termsUpdated),
		Routes: []web.Registrar{NewWeb(WebOpts{
			Kit:               layout.Kit{BasePath: base},
			Service:           NewService(f.users),
			Sessions:          sessions,
			RequireAcceptance: on,
		})},
	})
	return f
}

func (f *welcomeFixture) cookieFor(t *testing.T, p session.Principal) *http.Cookie {
	t.Helper()
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.store.Save(context.Background(), sid, p, time.Now().Add(time.Hour)))
	return &http.Cookie{Name: session.CookieName, Value: session.Sign(testSecret, sid)}
}

func (f *welcomeFixture) get(c *http.Cookie, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

func newPerson() session.Principal {
	return session.Principal{UserID: uuid.New(), Source: session.SourceLocal, Email: "ada@example.com"}
}

func TestWelcomeGate_Off(t *testing.T) {
	f := newWelcomeFixture(t, "", false)
	rec := f.get(f.cookieFor(t, newPerson()), "/console/")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, http.StatusNotFound, f.get(nil, "/welcome").Code)
}

func TestWelcomeGate_RedirectsUntilAccepted(t *testing.T) {
	for _, base := range []string{"", "/app"} {
		t.Run(base, func(t *testing.T) {
			f := newWelcomeFixture(t, base, true)
			c := f.cookieFor(t, newPerson())

			rec := f.get(c, base+"/console/o/acme?tab=keys")
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			loc, err := url.Parse(rec.Header().Get("Location"))
			require.NoError(t, err)
			assert.Equal(t, base+"/welcome", loc.Path)
			assert.Equal(t, base+"/console/o/acme?tab=keys", loc.Query().Get("return_to"))

			page := f.get(c, loc.String())
			assert.Equal(t, http.StatusOK, page.Code)
			assert.Contains(t, page.Body.String(), `name="accept_terms"`)
			assert.Contains(t, page.Body.String(), `href="`+base+`/terms"`)

			form := url.Values{"accept_terms": {"1"}, "return_to": {base + "/console/o/acme?tab=keys"}}
			req := httptest.NewRequest(http.MethodPost, base+"/welcome", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(c)
			post := httptest.NewRecorder()
			f.srv.ServeHTTP(post, req)
			require.Equal(t, http.StatusSeeOther, post.Code, post.Body.String())
			assert.Equal(t, base+"/console/o/acme?tab=keys", post.Header().Get("Location"))
			assert.Len(t, f.users.accepted, 1)

			fresh := cookieNamed(post, session.CookieName)
			require.NotNil(t, fresh, "the session is rotated with the accepted time")
			assert.Equal(t, http.StatusOK, f.get(fresh, base+"/console/").Code)
			oldSid, err := session.Verify(testSecret, c.Value)
			require.NoError(t, err)
			_, ok, err := f.store.Load(context.Background(), oldSid)
			require.NoError(t, err)
			assert.False(t, ok, "the old session is gone")
		})
	}
}

func TestWelcome_PostWithoutCheckbox(t *testing.T) {
	f := newWelcomeFixture(t, "", true)
	c := f.cookieFor(t, newPerson())
	req := httptest.NewRequest(http.MethodPost, "/welcome", strings.NewReader("return_to=/console/"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "alert-error")
	assert.Empty(t, f.users.accepted)
}

func TestWelcome_AnonymousGoesToLogin(t *testing.T) {
	f := newWelcomeFixture(t, "", true)
	rec := f.get(nil, "/welcome")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login?return_to=%2Fwelcome", rec.Header().Get("Location"))
}

func TestWelcomeGate_AsksAgainWhenTheTermsChange(t *testing.T) {
	f := newWelcomeFixture(t, "", true)
	before, after := newPerson(), newPerson()
	before.TermsAcceptedAt = termsUpdated.Add(-time.Hour)
	after.TermsAcceptedAt = termsUpdated.Add(time.Hour)

	rec := f.get(f.cookieFor(t, before), "/console/")
	assert.Equal(t, http.StatusSeeOther, rec.Code, "accepted before the terms changed")
	page := f.get(f.cookieFor(t, before), "/welcome?return_to=/console/")
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `name="accept_terms"`)

	assert.Equal(t, http.StatusOK, f.get(f.cookieFor(t, after), "/console/").Code, "accepted after")
	assert.Equal(t, http.StatusSeeOther, f.get(f.cookieFor(t, after), "/welcome").Code)
}

func TestWelcome_AlreadyAcceptedSkips(t *testing.T) {
	f := newWelcomeFixture(t, "", true)
	p := newPerson()
	p.TermsAcceptedAt = time.Now()
	rec := f.get(f.cookieFor(t, p), "/welcome?return_to=/console/x")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/console/x", rec.Header().Get("Location"))
}

func TestWelcomeGate_NeverGatesNonPeople(t *testing.T) {
	f := newWelcomeFixture(t, "", true)
	for name, p := range map[string]session.Principal{
		"basic":   {Source: session.SourceBasic, Operator: true},
		"api key": {Source: session.SourceAPIKey, KeyID: uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusOK, f.get(f.cookieFor(t, p), "/console/").Code)
		})
	}
	assert.Equal(t, http.StatusOK, f.get(nil, "/console/").Code, "anonymous is not gated")
}

func TestWelcomeGate_SkipList(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	gate := WelcomeGate("/app", true, termsUpdated)(ok)
	for _, path := range []string{
		"/app/welcome", "/app/login", "/app/logout", "/app/install", "/app/terms", "/app/privacy",
		"/app/static/app.css", "/healthz", "/readyz", "/app/api/x.v1.S/M", "/app/mcp", "/app/mcp/x",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(session.PrincipalInto(req.Context(), newPerson()))
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTeapot, rec.Code, path)
	}
	req := httptest.NewRequest(http.MethodGet, "/welcome", nil)
	req = req.WithContext(session.PrincipalInto(req.Context(), newPerson()))
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code, "a path outside the base is not skipped")
}

func TestWelcomeGate_SkipsInviteAndResetLinks(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	gate := WelcomeGate("/app", true, termsUpdated)(ok)
	for path, want := range map[string]int{
		"/app/invite":               http.StatusTeapot,
		"/app/invite/raw-token-123": http.StatusTeapot,
		"/app/invite/accept":        http.StatusTeapot,
		"/app/invitee":              http.StatusSeeOther,
		"/app/forgot-password":      http.StatusTeapot,
		"/app/reset-password":       http.StatusTeapot,
		"/app/reset-password/raw-1": http.StatusTeapot,
		"/app/reset-passwords":      http.StatusSeeOther,
		"/app/console/o/acme":       http.StatusSeeOther,
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(session.PrincipalInto(req.Context(), newPerson()))
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		assert.Equal(t, want, rec.Code, path)
		assert.NotContains(t, rec.Header().Get("Location"), "raw-token-123", "the token is never copied into another URL")
		assert.NotContains(t, rec.Header().Get("Location"), "raw-1", "the token is never copied into another URL")
	}
}

var testSecret = []byte("0123456789abcdef0123456789abcdef")

type noRefresh struct{}

func (noRefresh) Refresh(_ context.Context, p session.Principal) (session.Principal, error) {
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

type failingUsers struct{ err error }

func (f failingUsers) AcceptTerms(context.Context, uuid.UUID) (*user.User, error) { return nil, f.err }

type rotatorFunc func(p session.Principal) error

func (f rotatorFunc) Rotate(_ context.Context, _ http.ResponseWriter, _ *http.Request, p session.Principal) error {
	return f(p)
}

func postAccept(h *Web, p *session.Principal) *httptest.ResponseRecorder {
	form := url.Values{"accept_terms": {"1"}, "return_to": {"/console/"}}
	req := httptest.NewRequest(http.MethodPost, "/welcome", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if p != nil {
		req = req.WithContext(session.PrincipalInto(req.Context(), *p))
	}
	rec := httptest.NewRecorder()
	h.postWelcome(rec, req)
	return rec
}

func TestWelcome_PostAnonymousGoesToLogin(t *testing.T) {
	h := NewWeb(WebOpts{Service: NewService(&fakeUsers{}), Sessions: rotatorFunc(func(session.Principal) error { return nil })})
	rec := postAccept(h, nil)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login?return_to=%2Fwelcome", rec.Header().Get("Location"))
}

func TestWelcome_PostFailures(t *testing.T) {
	cases := []struct {
		name    string
		users   TermsAcceptor
		rotate  error
		wantMsg string
	}{
		{"accept fails", failingUsers{err: errors.New("db down")}, nil, "web: accept terms"},
		{"rotate fails", &fakeUsers{}, errors.New("store down"), "web: rotate session"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var reported string
			var rotated session.Principal
			h := NewWeb(WebOpts{
				Kit: layout.Kit{Unexpected: func(_ context.Context, msg string, _ error, _ ...any) *apperror.AppError {
					reported = msg
					return nil
				}},
				Service: NewService(tt.users),
				Sessions: rotatorFunc(func(p session.Principal) error {
					rotated = p
					return tt.rotate
				}),
			})
			p := newPerson()

			rec := postAccept(h, &p)

			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.Equal(t, tt.wantMsg, reported)
			assert.NotContains(t, rec.Body.String(), "down")
			if tt.rotate != nil {
				assert.False(t, rotated.TermsAcceptedAt.IsZero(), "the rotated session carries the acceptance time")
			}
		})
	}
}
