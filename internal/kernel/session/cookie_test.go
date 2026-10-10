package session

import (
	"encoding/base64"
	"testing"

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
