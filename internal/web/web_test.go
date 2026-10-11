package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthz(t *testing.T) {
	srv := NewServer(ServerOpts{})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestReadyz(t *testing.T) {
	ready := false
	srv := NewServer(ServerOpts{Ready: func() bool { return ready }})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	ready = true
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestProbesStayRootAnchoredUnderBasePath(t *testing.T) {
	srv := NewServer(ServerOpts{BasePath: "/app"})

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
}

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
	for _, base := range []string{"", "/app"} {
		srv := NewServer(ServerOpts{BasePath: base})

		rec := get(t, srv, base+"/static/app.css")
		assert.Equal(t, http.StatusOK, rec.Code, base)
		assert.Equal(t, "text/css; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		assert.NotEmpty(t, rec.Body.String())

		rec = get(t, srv, base+"/static/favicon.svg")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "image/svg+xml", rec.Header().Get("Content-Type"))

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
