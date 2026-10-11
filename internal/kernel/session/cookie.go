package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const idBytes = 32

// CookieName is the browser session cookie; its value is Sign(stateSecret, sid).
const CookieName = "oagent_sid"

// CookieOptions are the deployment-wide attributes of the session cookie.
type CookieOptions struct {
	Path   string
	Secure bool
}

// WriteCookie sets the session cookie to signed until expires; it is always HttpOnly and SameSite=Lax.
func WriteCookie(w http.ResponseWriter, c CookieOptions, signed string, expires time.Time) {
	http.SetCookie(w, c.cookie(signed, expires, 0))
}

// ClearCookie tells the browser to drop the session cookie.
func ClearCookie(w http.ResponseWriter, c CookieOptions) {
	http.SetCookie(w, c.cookie("", time.Unix(0, 0), -1))
}

// ReadCookie returns the session id of the first session cookie in h whose signature verifies.
func ReadCookie(h http.Header, secret []byte) (string, bool) {
	for _, raw := range sessionCookies(h) {
		if sid, err := Verify(secret, raw); err == nil {
			return sid, true
		}
	}
	return "", false
}

// HasCookie reports whether h carries a session cookie, valid or not.
func HasCookie(h http.Header) bool {
	return len(sessionCookies(h)) > 0
}

func sessionCookies(h http.Header) []string {
	var out []string
	for _, line := range h.Values("Cookie") {
		cs, err := http.ParseCookie(line)
		if err != nil {
			continue
		}
		for _, c := range cs {
			if c.Name == CookieName {
				out = append(out, c.Value)
			}
		}
	}
	return out
}

func (c CookieOptions) cookie(value string, expires time.Time, maxAge int) *http.Cookie {
	path := c.Path
	if path == "" {
		path = "/"
	}
	return &http.Cookie{ //nolint:gosec // Secure follows http.cookieSecure.
		Name:     CookieName,
		Value:    value,
		Path:     path,
		Expires:  expires,
		MaxAge:   maxAge,
		Secure:   c.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// NewID returns a fresh session id: 32 random bytes, base64url without padding.
func NewID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: new id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Sign returns "value|mac" where mac is base64url HMAC-SHA256(value) keyed on secret.
func Sign(secret []byte, value string) string {
	return value + "|" + base64.RawURLEncoding.EncodeToString(mac(secret, value))
}

// Verify checks a signed cookie in constant time and returns its value.
func Verify(secret []byte, raw string) (string, error) {
	i := strings.LastIndexByte(raw, '|')
	if i <= 0 || i == len(raw)-1 {
		return "", &BadCookieError{}
	}
	value := raw[:i]
	sig, err := base64.RawURLEncoding.DecodeString(raw[i+1:])
	if err != nil || !hmac.Equal(sig, mac(secret, value)) {
		return "", &BadCookieError{}
	}
	return value, nil
}

func mac(secret []byte, value string) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(value))
	return h.Sum(nil)
}
