package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignedCookie(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	c := SignedCookie{Name: "oagent_reset", Path: "/reset-password", Secret: []byte("0123456789abcdef0123456789abcdef"), Secure: true,
		Now: func() time.Time { return now }}

	rec := httptest.NewRecorder()
	c.Set(rec, "pub|123", now.Add(time.Hour))
	set := rec.Result().Cookies()
	require.Len(t, set, 1)
	got := set[0]
	assert.Equal(t, "oagent_reset", got.Name)
	assert.Equal(t, "/reset-password", got.Path)
	assert.Equal(t, 3600, got.MaxAge)
	assert.True(t, got.HttpOnly)
	assert.True(t, got.Secure)
	assert.Equal(t, http.SameSiteLaxMode, got.SameSite)

	req := httptest.NewRequest(http.MethodGet, "/reset-password", nil)
	req.AddCookie(got)
	v, ok := c.Get(req)
	assert.True(t, ok)
	assert.Equal(t, "pub|123", v)

	other := c
	other.Name = "oagent_invite"
	swapped := httptest.NewRequest(http.MethodGet, "/", nil)
	swapped.AddCookie(&http.Cookie{Name: "oagent_invite", Value: got.Value})
	_, ok = other.Get(swapped)
	assert.False(t, ok, "a value signed for one cookie is refused by another")

	tampered := httptest.NewRequest(http.MethodGet, "/", nil)
	tampered.AddCookie(&http.Cookie{Name: "oagent_reset", Value: "x" + got.Value})
	_, ok = c.Get(tampered)
	assert.False(t, ok)
	_, ok = c.Get(httptest.NewRequest(http.MethodGet, "/", nil))
	assert.False(t, ok)

	rec = httptest.NewRecorder()
	c.Set(rec, "late", now.Add(-time.Minute))
	assert.Equal(t, 1, rec.Result().Cookies()[0].MaxAge, "an expiry in the past still sets a one-second cookie")

	rec = httptest.NewRecorder()
	c.Clear(rec)
	cleared := rec.Result().Cookies()[0]
	assert.Equal(t, "oagent_reset", cleared.Name)
	assert.Equal(t, "/reset-password", cleared.Path)
	assert.Negative(t, cleared.MaxAge)

	rec = httptest.NewRecorder()
	SignedCookie{Name: "n", Secret: []byte("k")}.Set(rec, "v", time.Now().Add(time.Minute))
	assert.Equal(t, "/", rec.Result().Cookies()[0].Path, "the default path is /")
}
