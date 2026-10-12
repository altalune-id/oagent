package auth

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

type failingStore struct {
	*session.MemoryStore
	saveErr, deleteErr error
}

func (f failingStore) Save(ctx context.Context, sid string, p session.Principal, expiresAt time.Time) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.MemoryStore.Save(ctx, sid, p, expiresAt)
}

func (f failingStore) Delete(ctx context.Context, sid string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.MemoryStore.Delete(ctx, sid)
}

func TestCookieSessions_StartSaveFailsSetsNoCookie(t *testing.T) {
	s := testSessions(failingStore{MemoryStore: session.NewMemoryStore(), saveErr: errors.New("db down")})
	rec := httptest.NewRecorder()
	_, _, err := s.Start(context.Background(), rec, session.Principal{UserID: uuid.New(), Source: session.SourceLocal})
	require.EqualError(t, err, "db down")
	assert.Empty(t, rec.Result().Cookies())
}

func startedRequest(t *testing.T, s CookieSessions) (*http.Request, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	_, _, err := s.Start(context.Background(), rec, session.Principal{UserID: uuid.New(), Source: session.SourceLocal})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	sid, ok := s.Read(req)
	require.True(t, ok)
	return req, sid
}

func TestCookieSessions_RotateReplacesTheOldSession(t *testing.T) {
	store := session.NewMemoryStore()
	s := testSessions(store)
	req, old := startedRequest(t, s)
	p := session.Principal{UserID: uuid.New(), Source: session.SourceLocal, Email: "ada@example.com"}

	rec := httptest.NewRecorder()
	require.NoError(t, s.Rotate(context.Background(), rec, req, p))

	_, ok, err := store.Load(context.Background(), old)
	require.NoError(t, err)
	assert.False(t, ok, "the old session is deleted")

	cs := rec.Result().Cookies()
	require.Len(t, cs, 1)
	next := httptest.NewRequest(http.MethodGet, "/", nil)
	next.AddCookie(cs[0])
	sid, ok := s.Read(next)
	require.True(t, ok)
	assert.NotEqual(t, old, sid)
	got, ok, err := store.Load(context.Background(), sid)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, p.UserID, got.UserID)
}

func TestCookieSessions_RotateWithoutSessionStartsOne(t *testing.T) {
	s := testSessions(failingStore{MemoryStore: session.NewMemoryStore(), deleteErr: errors.New("must not delete")})
	rec := httptest.NewRecorder()
	require.NoError(t, s.Rotate(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), session.Principal{UserID: uuid.New()}))
	assert.Len(t, rec.Result().Cookies(), 1)
}

func TestCookieSessions_RotateErrors(t *testing.T) {
	tests := []struct {
		name    string
		store   failingStore
		wantErr string
	}{
		{name: "start", store: failingStore{saveErr: errors.New("save failed")}, wantErr: "save failed"},
		{name: "delete old", store: failingStore{deleteErr: errors.New("delete failed")}, wantErr: "delete failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := session.NewMemoryStore()
			req, old := startedRequest(t, testSessions(mem))
			tt.store.MemoryStore = mem

			err := testSessions(tt.store).Rotate(context.Background(), httptest.NewRecorder(), req, session.Principal{UserID: uuid.New()})
			require.EqualError(t, err, tt.wantErr)
			_, ok, err := mem.Load(context.Background(), old)
			require.NoError(t, err)
			assert.True(t, ok, "the old session survives a failed rotate")
		})
	}
}
