package auth

import (
	"context"
	"net/http"
	"time"

	"altalune.id/oagent/internal/kernel/session"
)

// CookieSessions starts, reads, rotates and ends browser sessions.
type CookieSessions struct {
	Store  session.Store
	Secret []byte
	Cookie session.CookieOptions
	TTL    time.Duration
}

// Start saves p under a new session id and sets the signed cookie; it returns the cookie value and expiry.
func (s CookieSessions) Start(ctx context.Context, w http.ResponseWriter, p session.Principal) (string, time.Time, error) {
	sid, err := session.NewID()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(s.TTL)
	if err := s.Store.Save(ctx, sid, p, expires); err != nil {
		return "", time.Time{}, err
	}
	signed := session.Sign(s.Secret, sid)
	session.WriteCookie(w, s.Cookie, signed, expires)
	return signed, expires, nil
}

// Read returns the session id from r's verified session cookie.
func (s CookieSessions) Read(r *http.Request) (string, bool) {
	return session.ReadCookie(r.Header, s.Secret)
}

// Rotate starts a new session for p and deletes the one r carried, if any.
func (s CookieSessions) Rotate(ctx context.Context, w http.ResponseWriter, r *http.Request, p session.Principal) error {
	old, hadSession := s.Read(r)
	if _, _, err := s.Start(ctx, w, p); err != nil {
		return err
	}
	if !hadSession {
		return nil
	}
	return s.Store.Delete(ctx, old)
}

// End deletes the session sid and clears the cookie.
func (s CookieSessions) End(ctx context.Context, w http.ResponseWriter, sid string) error {
	session.ClearCookie(w, s.Cookie)
	if sid == "" {
		return nil
	}
	return s.Store.Delete(ctx, sid)
}
