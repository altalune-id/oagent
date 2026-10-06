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
	App        http.Handler // optional app surface mounted under BasePath
	BasePath   string
	Log        *slog.Logger
	Unexpected apperror.UnexpectedFunc
	Ready      func() bool
}

// NewServer builds the root http.Handler: probes stay at the root, the app mounts under BasePath.
func NewServer(opts ServerOpts) http.Handler {
	outer := http.NewServeMux()
	outer.HandleFunc("GET /healthz", healthz)
	outer.HandleFunc("GET /readyz", readyz(opts.Ready))
	if opts.APIHandler != nil {
		outer.Handle(Path(opts.BasePath, "/api")+"/", opts.APIHandler)
	}
	if opts.MCPHandler != nil {
		outer.Handle(Path(opts.BasePath, "/mcp"), opts.MCPHandler)
		outer.Handle(Path(opts.BasePath, "/mcp")+"/", opts.MCPHandler)
	}

	app := opts.App
	if app == nil {
		app = http.NewServeMux()
	}
	mountApp(outer, opts.BasePath, app)

	return middleware.Recoverer(opts.Log, opts.Unexpected, middleware.RequestID(outer))
}

// mountApp serves app at BasePath, or at the root when BasePath is empty.
func mountApp(outer *http.ServeMux, basePath string, app http.Handler) {
	base := Path(basePath, "")
	if base == "/" {
		outer.Handle("/", app)
		return
	}
	outer.Handle("/", http.NotFoundHandler())
	outer.Handle(base+"/", http.StripPrefix(base, app))
	outer.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
	})
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
