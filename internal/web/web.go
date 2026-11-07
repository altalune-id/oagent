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
	DataPlane  http.Handler // optional bearer-only integrator API mounted at BasePath+"/api/v1", with no session or CSRF check
	Ingest     http.Handler // optional signed provider deliveries mounted at BasePath+"/hooks", with no session or CSRF check
	MCPHandler http.Handler // optional MCP surface mounted at BasePath+"/mcp"
	Console    http.Handler // optional console SPA mounted at BasePath+"/console"
	Routes     []Registrar  // optional server-rendered routes; their mux also answers every unknown path
	NotFound   http.Handler // optional page for paths no route claims; a wrong method on a real route keeps its 405
	Probes     Registrar    // optional /healthz and /readyz, mounted with no gate, session or CSRF check

	ResourceMetadata map[string]http.Handler // optional RFC 9728 documents by path, mounted like Probes

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
	for path, h := range opts.ResourceMetadata {
		outer.Handle("GET "+path, h)
	}
	if opts.APIHandler != nil {
		outer.Handle(layout.Path(opts.BasePath, "/api")+"/", cop.Handler(opts.APIHandler))
	}
	if opts.DataPlane != nil {
		outer.Handle(layout.Path(opts.BasePath, "/api/v1")+"/", opts.DataPlane)
	}
	if opts.Ingest != nil {
		outer.Handle(layout.Path(opts.BasePath, "/hooks")+"/", opts.Ingest)
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
	outer.Handle("GET "+static, staticHandler(static, loadStatic))
	outer.Handle("GET "+layout.Path(opts.BasePath, "/favicon.ico"), http.RedirectHandler(static+"favicon.ico", http.StatusFound))
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
		outer.Handle("/", middleware.NoStore(cop.Handler(withSession(withNotFound(routes, opts.NotFound)))))
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

// NOTE: only the mux's own 404 becomes the branded page; its 405 with Allow and every handler's answer pass through.
func withNotFound(mux *http.ServeMux, notFound http.Handler) http.Handler {
	if notFound == nil {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		probe := &notFoundProbe{ResponseWriter: w}
		mux.ServeHTTP(probe, r)
		if probe.missed {
			notFound.ServeHTTP(w, r)
		}
	})
}

type notFoundProbe struct {
	http.ResponseWriter
	missed, wrote bool
}

func (p *notFoundProbe) WriteHeader(code int) {
	if !p.wrote && code == http.StatusNotFound {
		p.missed, p.wrote = true, true
		return
	}
	p.wrote = true
	p.ResponseWriter.WriteHeader(code)
}

func (p *notFoundProbe) Write(b []byte) (int, error) {
	if !p.wrote {
		p.WriteHeader(http.StatusOK)
	}
	if p.missed {
		return len(b), nil
	}
	return p.ResponseWriter.Write(b)
}
