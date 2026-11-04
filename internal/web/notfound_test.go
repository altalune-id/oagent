package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func branded() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("branded"))
	})
}

func notFoundMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /gone", http.NotFound)
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestWithNotFound(t *testing.T) {
	h := withNotFound(notFoundMux(), branded())

	rec := serve(h, http.MethodGet, "/nope")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "branded", rec.Body.String(), "an unclaimed path gets the NotFound page")
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))

	rec = serve(h, http.MethodGet, "/logout")
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "a wrong method keeps its 405")
	assert.Equal(t, "POST", rec.Header().Get("Allow"))
	assert.NotContains(t, rec.Body.String(), "branded")

	rec = serve(h, http.MethodGet, "/gone")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "404 page not found\n", rec.Body.String(), "a matched route's own 404 passes through unbranded")

	rec = serve(h, http.MethodGet, "/ok")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String(), "a matched route's answer is untouched")

	mux := notFoundMux()
	assert.Same(t, mux, withNotFound(mux, nil), "no NotFound page leaves the mux as it is")
}

func TestNotFoundProbe(t *testing.T) {
	rec := httptest.NewRecorder()
	p := &notFoundProbe{ResponseWriter: rec}
	_, _ = p.Write([]byte("hi"))
	p.WriteHeader(http.StatusNotFound)
	assert.False(t, p.missed, "a 404 after the body started is not the mux's own")
	assert.Equal(t, "hi", rec.Body.String())

	rec = httptest.NewRecorder()
	p = &notFoundProbe{ResponseWriter: rec}
	p.WriteHeader(http.StatusNotFound)
	n, err := p.Write([]byte("404 page not found"))
	assert.NoError(t, err)
	assert.Equal(t, 18, n)
	assert.True(t, p.missed)
	assert.Empty(t, rec.Body.String(), "the mux's own 404 body is dropped for the branded page")
}

func TestNewServer_NotFoundNeedsRoutes(t *testing.T) {
	rec := serve(NewServer(ServerOpts{NotFound: branded()}), http.MethodGet, "/nope")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), "branded", "with no routes the outer mux answers its plain 404")

	rec = serve(NewServer(ServerOpts{Routes: []Registrar{okRoute{}}, NotFound: branded()}), http.MethodGet, "/nope")
	assert.Equal(t, "branded", rec.Body.String(), "NewServer wires NotFound behind the routes")
}

type okRoute struct{}

func (okRoute) Register(m *http.ServeMux) {
	m.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
}
