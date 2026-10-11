package auth

import (
	"context"
	"net/http"
	"net/url"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/web/layout"
)

const basicChallenge = `Basic realm="oagent"`

// BasicSignIn checks the shared basic-mode credentials; *Service implements it.
type BasicSignIn interface {
	Basic(ctx context.Context, user, pass string) (session.Principal, error)
}

// ConsoleGate lets only a signed-in caller into the console: local mode (basic nil) redirects to /login, basic mode challenges.
func ConsoleGate(basePath string, basic BasicSignIn) func(http.Handler) http.Handler {
	login := layout.Path(basePath, "/login")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if session.PrincipalFrom(r.Context()).IsPerson() {
				next.ServeHTTP(w, r)
				return
			}
			if basic == nil {
				to := login + "?return_to=" + url.QueryEscape(r.URL.RequestURI())
				http.Redirect(w, r, to, http.StatusSeeOther) //nolint:gosec // G710: a fixed path; the return_to is checked by /login.
				return
			}
			if user, pass, ok := r.BasicAuth(); ok {
				if p, err := basic.Basic(r.Context(), user, pass); err == nil {
					next.ServeHTTP(w, r.WithContext(session.PrincipalInto(r.Context(), p)))
					return
				}
			}
			w.Header().Set("WWW-Authenticate", basicChallenge)
			http.Error(w, "Sign in required", http.StatusUnauthorized)
		})
	}
}

// BasicChallenge answers a credential-less basic-mode /api request with a Basic challenge, so the browser prompts once.
func BasicChallenge(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || session.HasCookie(r.Header) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", basicChallenge)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "unauthenticated", "message": "Sign in required"})
	})
}
