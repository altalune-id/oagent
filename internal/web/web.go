// Package web builds the HTTP surface: the mux and its middleware.
package web

import (
	"log/slog"
	"net/http"

	"altalune.id/auth-server/apperror"
	"altalune.id/auth-server/internal/web/middleware"
)

// ServerOpts configures the web server handler.
type ServerOpts struct {
	Log        *slog.Logger
	Unexpected apperror.UnexpectedFunc
	Ready      func() bool
}

// NewServer builds the root http.Handler.
func NewServer(opts ServerOpts) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /readyz", readyz(opts.Ready))
	return middleware.Recoverer(opts.Log, opts.Unexpected, middleware.RequestID(mux))
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func readyz(ready func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if ready != nil && !ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
