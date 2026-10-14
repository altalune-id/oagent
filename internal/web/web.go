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
	Probes     Registrar    // optional /healthz and /readyz, mounted with no gate, session or CSRF check

	Gate        func(http.Handler) http.Handler // optional; wraps every route, probes included
	Session     func(http.Handler) http.Handler // optional; puts the cookie's Principal on routes and the console
	SessionGate func(http.Handler) http.Handler // optional; runs after Session on routes and the console
	ConsoleGate func(http.Handler) http.Handler // optional; runs after SessionGate on the console only

	CrossOrigin *http.CrossOriginProtection // guards routes and /api, not /mcp; nil uses the stdlib default
	BasePath    string
	Log         *slog.Logger
	Unexpected  apperror.UnexpectedFunc
}

// NewServer builds the root http.Handler.
func NewServer(opts ServerOpts) http.Handler {
	cop := opts.CrossOrigin
	if cop == nil {
		cop = http.NewCrossOriginProtection()
	}
	outer := http.NewServeMux()
	if opts.Probes != nil {
		opts.Probes.Register(outer)
	}
	if opts.APIHandler != nil {
		outer.Handle(layout.Path(opts.BasePath, "/api")+"/", cop.Handler(opts.APIHandler))
	}
	if opts.MCPHandler != nil {
		mcp := layout.Path(opts.BasePath, "/mcp")
		outer.Handle(mcp, opts.MCPHandler)
		outer.Handle(mcp+"/", opts.MCPHandler)
	}
	withSession := chain(opts.Session, opts.SessionGate)
	if opts.Console != nil {
		console := layout.Path(opts.BasePath, "/console")
		outer.Handle(console, http.RedirectHandler(console+"/", http.StatusMovedPermanently))
		spa := http.StripPrefix(console, opts.Console)
		if opts.ConsoleGate != nil {
			spa = opts.ConsoleGate(spa)
		}
		outer.Handle(console+"/", withSession(spa))
	}
	static := layout.Path(opts.BasePath, "/static") + "/"
	outer.Handle("GET "+static, staticHandler(static, StaticFS()))
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
		outer.Handle("/", cop.Handler(withSession(routes)))
	} else {
		outer.Handle("/", http.NotFoundHandler())
	}
	mcpPrefix := layout.Path(opts.BasePath, "/mcp")
	secured := middleware.SecurityHeaders(outer)
	var root http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mcpPrefix || strings.HasPrefix(r.URL.Path, mcpPrefix+"/") {
			outer.ServeHTTP(w, r)
			return
		}
		secured.ServeHTTP(w, r)
	})
	if opts.Gate != nil {
		root = opts.Gate(root)
	}
	return middleware.Recoverer(opts.Log, opts.Unexpected, middleware.RequestID(root))
}

func chain(outer, inner func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		if inner != nil {
			h = inner(h)
		}
		if outer != nil {
			h = outer(h)
		}
		return h
	}
}
