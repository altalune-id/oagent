// Package web builds the HTTP surface: the mux and its middleware.
package web

import (
	"log/slog"
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/web/middleware"
)

// ServerOpts configures the web server handler.
type ServerOpts struct {
	APIHandler http.Handler // optional Connect-RPC surface mounted at BasePath+"/api"
	MCPHandler http.Handler // optional MCP surface mounted at BasePath+"/mcp"
	Console    http.Handler // optional console SPA mounted at BasePath+"/console"
	BasePath   string
	Log        *slog.Logger
	Unexpected apperror.UnexpectedFunc
	Ready      func() bool
}

// NewServer builds the root http.Handler.
func NewServer(opts ServerOpts) http.Handler {
	outer := http.NewServeMux()
	outer.HandleFunc("GET /healthz", healthz)
	outer.HandleFunc("GET /readyz", readyz(opts.Ready))
	if opts.APIHandler != nil {
		outer.Handle(Path(opts.BasePath, "/api")+"/", opts.APIHandler)
	}
	if opts.MCPHandler != nil {
		mcp := Path(opts.BasePath, "/mcp")
		outer.Handle(mcp, opts.MCPHandler)
		outer.Handle(mcp+"/", opts.MCPHandler)
	}
	if opts.Console != nil {
		console := Path(opts.BasePath, "/console")
		outer.Handle(console, http.RedirectHandler(console+"/", http.StatusMovedPermanently))
		outer.Handle(console+"/", http.StripPrefix(console, opts.Console))
	}
	outer.Handle("/", http.NotFoundHandler())
	return middleware.Recoverer(opts.Log, opts.Unexpected, middleware.RequestID(outer))
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
