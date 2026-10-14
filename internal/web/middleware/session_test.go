package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

type refresher func(session.Principal) (session.Principal, error)

func (f refresher) Refresh(_ context.Context, p session.Principal) (session.Principal, error) {
	return f(p)
}

type sessionFixture struct {
	store    *session.MemoryStore
	reported []error
	seen     session.Principal
	seenSID  string
	handler  http.Handler
}

func newSessionFixture(t *testing.T, r refresher) *sessionFixture {
	t.Helper()
	f := &sessionFixture{store: session.NewMemoryStore()}
	unexpected := func(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
		f.reported = append(f.reported, cause)
		return apperror.New(apperror.CodeUnexpected, msg)
	}
	next := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		f.seen = session.PrincipalFrom(req.Context())
		f.seenSID, _ = session.SIDFrom(req.Context())
	})
	f.handler = Session(SessionConfig{Store: f.store, Secret: secret, Auth: r, Unexpected: unexpected})(next)
	return f
}

func (f *sessionFixture) serve(cookie string) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: cookie})
	}
	f.handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestSession_RefreshesAuthority(t *testing.T) {
	stored := session.Principal{UserID: uuid.New(), Source: session.SourceLocal, Scopes: []string{"*"}}
	role := []string{"*"}
	f := newSessionFixture(t, func(p session.Principal) (session.Principal, error) {
		p.Scopes = role
		return p, nil
	})
	require.NoError(t, f.store.Save(context.Background(), "sid-1", stored, time.Now().Add(time.Hour)))

	f.serve(session.Sign(secret, "sid-1"))
	assert.Equal(t, stored.UserID, f.seen.UserID)
	assert.Equal(t, []string{"*"}, f.seen.Scopes)

	role = []string{"agent:read"}
	f.serve(session.Sign(secret, "sid-1"))
	assert.Equal(t, []string{"agent:read"}, f.seen.Scopes, "a role change shows on the next request")
}

func TestSession_AnonymousCases(t *testing.T) {
	ok := func(p session.Principal) (session.Principal, error) { return p, nil }
	cases := map[string]string{
		"no cookie":      "",
		"bad signature":  session.Sign([]byte("another-secret-another-secret-xx"), "sid-1"),
		"unknown sid":    session.Sign(secret, "missing"),
		"garbage cookie": "%%%",
	}
	for name, cookie := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSessionFixture(t, ok)
			f.serve(cookie)
			assert.Empty(t, f.seen.Source)
			assert.Empty(t, f.reported)
		})
	}
}

func TestSession_RefreshErrorIsAnonymousAndReported(t *testing.T) {
	f := newSessionFixture(t, func(session.Principal) (session.Principal, error) { return session.Principal{}, errors.New("db down") })
	require.NoError(t, f.store.Save(context.Background(), "sid-1", session.Principal{UserID: uuid.New(), Source: session.SourceLocal}, time.Now().Add(time.Hour)))

	f.serve(session.Sign(secret, "sid-1"))
	assert.Empty(t, f.seen.Source)
	require.Len(t, f.reported, 1)
	assert.ErrorContains(t, f.reported[0], "db down")
}

type notFound struct{}

func (notFound) Error() string { return "gone" }
func (e notFound) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserNotFound, "gone").WithCause(e)
}

func TestSession_RefreshNotFoundIsNotReported(t *testing.T) {
	f := newSessionFixture(t, func(session.Principal) (session.Principal, error) { return session.Principal{}, notFound{} })
	require.NoError(t, f.store.Save(context.Background(), "sid-1", session.Principal{UserID: uuid.New(), Source: session.SourceLocal}, time.Now().Add(time.Hour)))

	f.serve(session.Sign(secret, "sid-1"))
	assert.Empty(t, f.seen.Source)
	assert.Empty(t, f.reported)
}

func TestSession_PutsTheSIDOnTheContext(t *testing.T) {
	f := newSessionFixture(t, func(p session.Principal) (session.Principal, error) { return p, nil })
	p := session.Principal{UserID: uuid.New(), Source: session.SourceLocal}
	require.NoError(t, f.store.Save(context.Background(), "sid-1", p, time.Now().Add(time.Hour)))
	f.serve(session.Sign(secret, "sid-1"))
	assert.Equal(t, "sid-1", f.seenSID)

	f.serve(session.Sign(secret, "sid-gone"))
	assert.Empty(t, f.seenSID, "no sid for a session that is not stored")
	f.serve("")
	assert.Empty(t, f.seenSID)
}
