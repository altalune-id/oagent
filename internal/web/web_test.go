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

func TestRootStaysNotFoundWithConsole(t *testing.T) {
	app := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	srv := NewServer(ServerOpts{Console: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
