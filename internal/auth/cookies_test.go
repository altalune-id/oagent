package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func testSessions(store session.Store) CookieSessions {
	return CookieSessions{
		Store:  store,
		Secret: testSecret,
		Cookie: session.CookieOptions{Path: "/app", Secure: true},
		TTL:    time.Hour,
	}
}

func TestCookieSessions_StartAndEnd(t *testing.T) {
	store := session.NewMemoryStore()
	s := testSessions(store)
	p := session.Principal{UserID: uuid.New(), Source: session.SourceLocal, Email: "ada@example.com"}

	rec := httptest.NewRecorder()
	signed, expires, err := s.Start(context.Background(), rec, p)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Hour), expires, time.Minute)

	cs := rec.Result().Cookies()
	require.Len(t, cs, 1)
	assert.Equal(t, session.CookieName, cs[0].Name)
	assert.Equal(t, signed, cs[0].Value)
	assert.Equal(t, "/app", cs[0].Path)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cs[0])
	sid, ok := s.Read(req)
	require.True(t, ok)
	got, ok, err := store.Load(context.Background(), sid)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, p.UserID, got.UserID)

	rec = httptest.NewRecorder()
	require.NoError(t, s.End(context.Background(), rec, sid))
	_, ok, err = store.Load(context.Background(), sid)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, -1, rec.Result().Cookies()[0].MaxAge)
}

func TestCookieSessions_ReadRejectsTamperedCookie(t *testing.T) {
	s := testSessions(session.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: session.Sign([]byte("another-secret-another-secret-xx"), "sid")})
	_, ok := s.Read(req)
	assert.False(t, ok)
}
