package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrossOriginProtection_APIAndPagesNotMCP(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	cop := http.NewCrossOriginProtection()
	require.NoError(t, cop.AddTrustedOrigin("https://oagent.example.com"))
	srv := NewServer(ServerOpts{APIHandler: ok, MCPHandler: ok, Routes: []Registrar{loginRoute{ok}}, CrossOrigin: cop})

	post := func(path string, headers map[string]string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Host = "internal:8080"
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}
	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site"}
	for _, path := range []string{"/api/x.v1.S/M", "/login"} {
		assert.Equal(t, http.StatusForbidden, post(path, crossSite), path)
		assert.Equal(t, http.StatusForbidden, post(path, map[string]string{"Origin": "https://evil.example"}), path)
		assert.Equal(t, http.StatusTeapot, post(path, nil), path+": no browser headers (CLI) pass")
		assert.Equal(t, http.StatusTeapot, post(path, map[string]string{"Sec-Fetch-Site": "same-origin"}), path)
		assert.Equal(t, http.StatusTeapot, post(path, map[string]string{"Origin": "https://oagent.example.com"}), path+": baseURL origin is trusted")
	}
	assert.Equal(t, http.StatusTeapot, post("/mcp", crossSite), "/mcp is not wrapped")
}

type loginRoute struct{ h http.Handler }

func (l loginRoute) Register(mux *http.ServeMux) { mux.Handle("POST /login", l.h) }

func TestConsoleRoutes_BehindCrossOriginAndSessionNotTheSPA(t *testing.T) {
	route := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Session", r.Header.Get("X-Session-Seen"))
		w.WriteHeader(http.StatusTeapot)
	})
	spa := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	sessioned := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Session-Seen", "yes")
			next.ServeHTTP(w, r)
		})
	}
	srv := NewServer(ServerOpts{
		Console: spa, Session: sessioned, BasePath: "/app",
		ConsoleRoutes: map[string]http.Handler{"POST /agents/{agent}/try/runs": route},
	})
	do := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	same := do(http.MethodPost, "/app/console/agents/A1/try/runs", map[string]string{"Sec-Fetch-Site": "same-origin"})
	assert.Equal(t, http.StatusTeapot, same.Code)
	assert.Equal(t, "yes", same.Header().Get("X-Session"), "the session chain runs first")
	assert.Equal(t, http.StatusForbidden, do(http.MethodPost, "/app/console/agents/A1/try/runs", map[string]string{"Sec-Fetch-Site": "cross-site"}).Code,
		"a cross-site POST is refused")
	assert.Equal(t, http.StatusOK, do(http.MethodGet, "/app/console/agents/A1", nil).Code, "every other console path is still the SPA")
}
