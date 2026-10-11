// Package web builds the HTTP surface: the mux and its middleware.
package web

import (
	"log/slog"
	"net/http"
	"strings"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

// Registrar mounts a domain's server-rendered routes; each domain's web.go implements it.
type Registrar interface {
	Register(mux *http.ServeMux)
}

// ServerOpts configures the web server handler.
type ServerOpts struct {
	APIHandler http.Handler // optional Connect-RPC surface mounted at BasePath+"/api"
	MCPHandler http.Handler // optional MCP surface mounted at BasePath+"/mcp"
	Console    http.Handler // optional console SPA mounted at BasePath+"/console"
	Routes     []Registrar  // optional server-rendered routes; their mux also answers every unknown path

	Gate func(http.Handler) http.Handler // optional; wraps every route, probes included

	BasePath   string
	Log        *slog.Logger
	Unexpected apperror.UnexpectedFunc
	Ready      func() bool
}

// NewServer builds the root http.Handler.
func NewServer(opts ServerOpts) http.Handler {
	cop := http.NewCrossOriginProtection()
	outer := http.NewServeMux()
	outer.HandleFunc("GET /healthz", healthz)
	outer.HandleFunc("GET /readyz", readyz(opts.Ready))
	if opts.APIHandler != nil {
		outer.Handle(layout.Path(opts.BasePath, "/api")+"/", opts.APIHandler)
	}
	if opts.MCPHandler != nil {
		mcp := layout.Path(opts.BasePath, "/mcp")
		outer.Handle(mcp, opts.MCPHandler)
		outer.Handle(mcp+"/", opts.MCPHandler)
	}
	if opts.Console != nil {
		console := layout.Path(opts.BasePath, "/console")
		outer.Handle(console, http.RedirectHandler(console+"/", http.StatusMovedPermanently))
		outer.Handle(console+"/", http.StripPrefix(console, opts.Console))
	}
	static := layout.Path(opts.BasePath, "/static") + "/"
	outer.Handle("GET "+static, staticHandler(static))
	toConsole := http.RedirectHandler(layout.Path(opts.BasePath, "/console")+"/", http.StatusFound)
	outer.Handle("GET "+strings.TrimRight(opts.BasePath, "/")+"/{$}", toConsole)
	if bp := strings.TrimRight(opts.BasePath, "/"); bp != "" {
		outer.Handle("GET "+bp, toConsole)
	}
	if len(opts.Routes) > 0 {
		routes := http.NewServeMux()
		for _, r := range opts.Routes {
			r.Register(routes)
		}
		outer.Handle("/", cop.Handler(routes))
	} else {
		outer.Handle("/", http.NotFoundHandler())
	}
	var root http.Handler = outer
	if opts.Gate != nil {
		root = opts.Gate(outer)
	}
	return middleware.Recoverer(opts.Log, opts.Unexpected, middleware.RequestID(root))
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
