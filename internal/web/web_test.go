package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIHandlerMountsUnderBasePath(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := NewServer(ServerOpts{BasePath: "/app", APIHandler: api})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/api/org.v1.OrgService/CreateOrg", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/app/api/org.v1.OrgService/CreateOrg", rec.Body.String())
}

func TestMCPHandlerMountsUnderBasePath(t *testing.T) {
	mcp := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := NewServer(ServerOpts{BasePath: "/app", MCPHandler: mcp})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/mcp", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/app/mcp", rec.Body.String())
}

func TestConsoleMountsAtRootConsole(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := NewServer(ServerOpts{Console: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console/o/acme", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/o/acme", rec.Body.String())

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/console", nil))
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/console/", rec.Header().Get("Location"))
}

func TestConsoleMountsUnderBasePath(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := NewServer(ServerOpts{BasePath: "/app", Console: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/console/ping", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/ping", rec.Body.String())
}

func TestUnknownPathStaysNotFoundWithConsole(t *testing.T) {
	app := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	srv := NewServer(ServerOpts{Console: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/elsewhere", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

type noRoutes struct{}

func (noRoutes) Register(*http.ServeMux) {}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRoutes_UnknownPathIs404(t *testing.T) {
	srv := NewServer(ServerOpts{Routes: []Registrar{noRoutes{}}})
	assert.Equal(t, http.StatusNotFound, get(t, srv, "/nope").Code)
}

func TestStaticAssets(t *testing.T) {
	builtAsset(t, "app.css")
	builtAsset(t, "password.js")
	for _, base := range []string{"", "/app"} {
		srv := NewServer(ServerOpts{BasePath: base})

		rec := get(t, srv, base+"/static/app.css")
		assert.Equal(t, http.StatusOK, rec.Code, base)
		assert.Equal(t, "text/css; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		assert.NotEmpty(t, rec.Body.String())

		rec = get(t, srv, base+"/static/favicon.ico")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, []string{"image/x-icon", "image/vnd.microsoft.icon"}, rec.Header().Get("Content-Type"))

		rec = get(t, srv, base+"/static/altalune-logo.png")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))

		rec = get(t, srv, base+"/static/password.js")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))

		assert.Equal(t, http.StatusNotFound, get(t, srv, base+"/static/missing.css").Code)
	}
}

func TestRootRedirectsToConsole(t *testing.T) {
	cases := map[string]struct{ base, path, want string }{
		"root":           {"", "/", "/console/"},
		"base with dash": {"/app", "/app/", "/app/console/"},
		"base bare":      {"/app", "/app", "/app/console/"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := get(t, NewServer(ServerOpts{BasePath: tc.base}), tc.path)
			assert.Equal(t, http.StatusFound, rec.Code)
			assert.Equal(t, tc.want, rec.Header().Get("Location"))
		})
	}
}

func TestRootRedirectIsGetOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	NewServer(ServerOpts{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.NotEqual(t, http.StatusFound, rec.Code)
}

func tag(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Trace", name)
			next.ServeHTTP(w, r)
		})
	}
}

type probes struct{}

func (probes) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
}

type pageRoute struct{}

func (pageRoute) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/page", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("page")) })
}

func TestMiddlewareOrder(t *testing.T) {
	console := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("console")) })
	srv := NewServer(ServerOpts{
		BasePath:    "/app",
		Console:     console,
		Routes:      []Registrar{pageRoute{}},
		Probes:      probes{},
		Gate:        tag("gate"),
		Session:     tag("session"),
		SessionGate: tag("session-gate"),
		ConsoleGate: tag("console-gate"),
	})
	cases := []struct {
		name  string
		path  string
		body  string
		trace []string
	}{
		{"probe skips session", "/healthz", "ok", []string{"gate"}},
		{"route runs session then its gate", "/app/page", "page", []string{"gate", "session", "session-gate"}},
		{"console adds the console gate last", "/app/console/x", "console", []string{"gate", "session", "session-gate", "console-gate"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, srv, tt.path)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.body, rec.Body.String())
			assert.Equal(t, tt.trace, rec.Header().Values("X-Trace"))
		})
	}
}

func TestChain(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	cases := []struct {
		name         string
		outer, inner func(http.Handler) http.Handler
		want         []string
	}{
		{"neither", nil, nil, nil},
		{"outer only", tag("outer"), nil, []string{"outer"}},
		{"inner only", nil, tag("inner"), []string{"inner"}},
		{"both, outer first", tag("outer"), tag("inner"), []string{"outer", "inner"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, chain(tt.outer, tt.inner)(ok), "/")
			assert.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, tt.want, rec.Header().Values("X-Trace"))
		})
	}
}

// SECURITY: pages and the console can't be framed by another site; /mcp is left alone because MCP hosts frame its views.
func TestSecurityHeadersOnEverySurfaceButMCP(t *testing.T) {
	ok := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	srv := NewServer(ServerOpts{
		BasePath: "/app", APIHandler: ok, MCPHandler: ok, Console: ok,
		Routes: []Registrar{registrarFunc(func(mux *http.ServeMux) { mux.Handle("GET /app/login", ok) })},
	})
	for _, p := range []string{"/app/login", "/app/console/", "/app/static/app.css", "/app/api/x.v1.S/M", "/app/nope"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"), p)
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), p)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/mcp", nil))
	assert.Empty(t, rec.Header().Get("X-Frame-Options"), "/mcp")
	assert.Empty(t, rec.Header().Get("Content-Security-Policy"), "/mcp")
}

type registrarFunc func(mux *http.ServeMux)

func (f registrarFunc) Register(mux *http.ServeMux) { f(mux) }
