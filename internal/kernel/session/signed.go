package session

import (
	"net/http"
	"strings"
	"time"
)

// SignedCookie is an HttpOnly, SameSite=Lax cookie whose value is signed with Secret and bound to its Name.
type SignedCookie struct {
	Name   string
	Path   string
	Secret []byte
	Secure bool
	Now    func() time.Time
}

// Set writes value, signed, until expires (at least one second).
func (c SignedCookie) Set(w http.ResponseWriter, value string, expires time.Time) {
	maxAge := max(int(expires.Sub(c.now()).Seconds()), 1)
	http.SetCookie(w, c.cookie(Sign(c.Secret, c.Name+"="+value), maxAge))
}

// Get returns the cookie's value when its signature verifies for this cookie's name.
func (c SignedCookie) Get(r *http.Request) (string, bool) {
	raw, err := r.Cookie(c.Name)
	if err != nil {
		return "", false
	}
	signed, err := Verify(c.Secret, raw.Value)
	if err != nil {
		return "", false
	}
	return strings.CutPrefix(signed, c.Name+"=")
}

// Clear tells the browser to drop the cookie.
func (c SignedCookie) Clear(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie("", -1))
}

func (c SignedCookie) cookie(value string, maxAge int) *http.Cookie {
	path := c.Path
	if path == "" {
		path = "/"
	}
	return &http.Cookie{ //nolint:gosec // Secure follows http.cookieSecure.
		Name: c.Name, Value: value, Path: path, MaxAge: maxAge,
		Secure: c.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}

func (c SignedCookie) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}
