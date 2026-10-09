package console

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":   {Data: []byte(`<script>window.__NUXT__.config={public:{apiUrl:"",appVersion:""},app:{baseURL:"/",buildAssetsDir:"/_nuxt/"}}</script><script src="/_nuxt/app.js"></script>`)},
		"_nuxt/app.js": {Data: []byte("console.log(1)")},
	}
}

func newSPA(fsys fstest.MapFS) *spa {
	return &spa{fs: fsys, base: "/console/", apiBase: "/api", appVersion: "1.2.3"}
}

func TestSPA(t *testing.T) {
	t.Parallel()

	t.Run("root serves rebased index", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		newSPA(testFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"/console/_nuxt/`)
		assert.Contains(t, rec.Body.String(), `baseURL:"/console/"`)
		assert.Contains(t, rec.Body.String(), `apiUrl:"/api"`)
		assert.Contains(t, rec.Body.String(), `appVersion:"1.2.3"`)
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	})

	t.Run("serves an existing asset", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		newSPA(testFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_nuxt/app.js", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "console.log")
	})

	t.Run("falls back to index for a client route", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		newSPA(testFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/o/acme/projects/1", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `baseURL:"/console/"`)
	})

	t.Run("missing index returns 404", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		newSPA(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/o/acme", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestRebaseNoopAtRoot(t *testing.T) {
	t.Parallel()
	in := `baseURL:"/"`
	assert.Equal(t, in, (&spa{base: "/"}).rebase(in))
}

func TestHandlerReturnsHandler(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Handler("/console/", "/api", "1.2.3"))
}
