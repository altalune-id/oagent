package session

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCookie_SignVerify(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	signed := Sign(secret, "sid-value")
	got, err := Verify(secret, signed)
	require.NoError(t, err)
	assert.Equal(t, "sid-value", got)
}

func TestCookie_Rejects(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	signed := Sign(secret, "sid-value")
	for name, raw := range map[string]string{
		"empty":          "",
		"no separator":   "sid-value",
		"empty value":    "|abc",
		"empty sig":      "sid-value|",
		"tampered value": "sid-valuX" + signed[len("sid-value"):],
		"tampered sig":   signed[:len(signed)-2] + "AA",
		"bad base64":     "sid-value|!!!",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Verify(secret, raw)
			assert.True(t, IsBadCookieError(err), "got %v", err)
		})
	}
	t.Run("wrong secret", func(t *testing.T) {
		_, err := Verify([]byte("another-secret-another-secret-xx"), signed)
		assert.True(t, IsBadCookieError(err))
	})
}

func TestNewID(t *testing.T) {
	a, err := NewID()
	require.NoError(t, err)
	b, err := NewID()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	raw, err := base64.RawURLEncoding.DecodeString(a)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
}

func TestBadCookieError_Message(t *testing.T) {
	assert.Equal(t, "session: cookie signature invalid", (&BadCookieError{}).Error())
}

func TestWriteCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	exp := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	WriteCookie(rec, CookieOptions{Path: "/oagent", Secure: true}, "sid|mac", exp)

	cs := rec.Result().Cookies()
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, CookieName, c.Name)
	assert.Equal(t, "sid|mac", c.Value)
	assert.Equal(t, "/oagent", c.Path)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.True(t, exp.Equal(c.Expires))
}

func TestWriteCookie_EmptyPathIsRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteCookie(rec, CookieOptions{}, "v", time.Now().Add(time.Hour))
	assert.Equal(t, "/", rec.Result().Cookies()[0].Path)
}

func TestClearCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearCookie(rec, CookieOptions{Path: "/oagent"})
	c := rec.Result().Cookies()[0]
	assert.Equal(t, CookieName, c.Name)
	assert.Empty(t, c.Value)
	assert.Equal(t, -1, c.MaxAge)
	assert.Equal(t, "/oagent", c.Path)
	assert.True(t, c.HttpOnly)
}

func TestReadCookie(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	good := Sign(secret, "sid-1")
	cases := map[string]struct {
		headers []string
		sid     string
		ok      bool
	}{
		"missing":        {nil, "", false},
		"other cookie":   {[]string{"a=b"}, "", false},
		"good":           {[]string{"a=b; " + CookieName + "=" + good}, "sid-1", true},
		"tampered":       {[]string{CookieName + "=" + good + "x"}, "", false},
		"second header":  {[]string{"a=b", CookieName + "=" + good}, "sid-1", true},
		"bad then good":  {[]string{CookieName + "=junk; " + CookieName + "=" + good}, "sid-1", true},
		"malformed line": {[]string{"===;;"}, "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tc.headers {
				h.Add("Cookie", v)
			}
			sid, ok := ReadCookie(h, secret)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.sid, sid)
		})
	}
}

func TestHasCookie(t *testing.T) {
	h := http.Header{}
	assert.False(t, HasCookie(h))
	h.Add("Cookie", "a=b")
	assert.False(t, HasCookie(h))
	h.Add("Cookie", CookieName+"=junk")
	assert.True(t, HasCookie(h))
}
