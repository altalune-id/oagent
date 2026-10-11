package boot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/capabilities"
)

func TestHTTPConfig(t *testing.T) {
	c := httpConfig(config.HTTPConfig{Addr: ":8080"})
	assert.Equal(t, "http", c.Name)
	assert.Equal(t, ":8080", c.Addr)
}

func TestBuildHTTPHandler_ServesPages(t *testing.T) {
	cfg := config.Config{HTTP: config.HTTPConfig{BasePath: "/app"}}
	h := buildHTTPHandler(httpSurfaces{basePath: "/app", routes: buildRoutes(cfg, nil, nil, nil)})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/terms", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/", nil))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func serveRoutes(cfg config.Config, svcs *Services, method, path string) *httptest.ResponseRecorder {
	h := buildHTTPHandler(httpSurfaces{basePath: cfg.HTTP.BasePath, routes: buildRoutes(cfg, svcs, nil, nil)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestBuildRoutes_TermsOverride(t *testing.T) {
	cfg := config.Config{Compliance: config.ComplianceConfig{TermsURL: "https://example.com/t"}}
	rec := serveRoutes(cfg, nil, http.MethodGet, "/terms")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://example.com/t", rec.Header().Get("Location"))
}

func TestBuildRoutes_LoginOnlyWithLocalLogin(t *testing.T) {
	cfg := config.Config{}
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodGet, "/login").Code)
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodPost, "/logout").Code)

	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, Auth: &auth.Service{}}
	assert.Equal(t, http.StatusOK, serveRoutes(cfg, svcs, http.MethodGet, "/login").Code)
}
