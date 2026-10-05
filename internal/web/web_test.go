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

func TestBareBasePathRedirects(t *testing.T) {
	srv := NewServer(ServerOpts{BasePath: "/app/"})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))

	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/app/", rec.Header().Get("Location"))
}

func TestBasePathMountsAppWithStrippedPrefix(t *testing.T) {
	app := http.NewServeMux()
	app.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	srv := NewServer(ServerOpts{BasePath: "/app", App: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/ping", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/ping", rec.Body.String())

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
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

func TestRootAppWithEmptyBasePath(t *testing.T) {
	app := http.NewServeMux()
	app.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	srv := NewServer(ServerOpts{App: app})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
}
