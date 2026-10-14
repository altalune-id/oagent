package console

import (
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexHTML = `<script>window.__NUXT__.config={public:{apiUrl:"",appVersion:""},app:{baseURL:"/",buildAssetsDir:"/_nuxt/"}}</script><script src="/_nuxt/app.js"></script>`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html.gz":            {Data: []byte("GZ")},
		"index.html":               {Data: []byte(indexHTML + "<!--" + strings.Repeat("pad ", 300) + "-->")},
		"200.html":                 {Data: []byte("<html>200</html>")},
		"_nuxt/app.js":             {Data: []byte("console.log(1)")},
		"_nuxt/app.js.br":          {Data: []byte("BR")},
		"_nuxt/app.js.gz":          {Data: []byte("GZ")},
		"_nuxt/builds/latest.json": {Data: []byte(`{"id":"b1"}`)},
	}
}

func get(h http.Handler, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSPA(t *testing.T) {
	t.Parallel()
	h := newSPA(testFS(), "/console/", "/api", "1.2.3")

	t.Run("root serves rebased index", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"/console/_nuxt/`)
		assert.Contains(t, rec.Body.String(), `baseURL:"/console/"`)
		assert.Contains(t, rec.Body.String(), `apiUrl:"/api"`)
		assert.Contains(t, rec.Body.String(), `appVersion:"1.2.3"`)
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.NotEmpty(t, rec.Header().Get("ETag"))
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
	})

	t.Run("index revalidates to 304", func(t *testing.T) {
		t.Parallel()
		etag := get(h, "/", nil).Header().Get("ETag")
		rec := get(h, "/o/acme", http.Header{"If-None-Match": {etag}})
		assert.Equal(t, http.StatusNotModified, rec.Code)
	})

	t.Run("index is gzipped for clients that accept it", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/", http.Header{"Accept-Encoding": {"br, gzip"}})
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
		assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		body, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.Contains(t, string(body), `baseURL:"/console/"`)
	})

	t.Run("hashed asset is immutable and served as brotli", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/_nuxt/app.js", http.Header{"Accept-Encoding": {"gzip, br"}})
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "BR", rec.Body.String())
		assert.Equal(t, "br", rec.Header().Get("Content-Encoding"))
		assert.Equal(t, "text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	})

	t.Run("plain asset without accept-encoding", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/_nuxt/app.js", nil)
		assert.Equal(t, "console.log(1)", rec.Body.String())
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
	})

	t.Run("range on an asset gets plain bytes", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/_nuxt/app.js", http.Header{"Accept-Encoding": {"br"}, "Range": {"bytes=0-6"}})
		assert.Equal(t, http.StatusPartialContent, rec.Code)
		assert.Equal(t, "console", rec.Body.String())
	})

	t.Run("build manifest is revalidated", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/_nuxt/builds/latest.json", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	})

	t.Run("root file has a short cache", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/200.html", nil)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		assert.NotEmpty(t, rec.Header().Get("ETag"))
	})

	t.Run("compressed sibling is not served by its own name", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, http.StatusNotFound, get(h, "/_nuxt/app.js.br", nil).Code)
		assert.Equal(t, http.StatusNotFound, get(h, "/_nuxt/app.js.gz", nil).Code)
		assert.Equal(t, http.StatusNotFound, get(h, "/index.html.gz", nil).Code)
	})

	t.Run("index.html redirects to the directory", func(t *testing.T) {
		t.Parallel()
		rec := get(h, "/index.html", nil)
		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
		assert.Equal(t, "./", rec.Header().Get("Location"))
		assert.Equal(t, "./?a=1", get(h, "/index.html?a=1", nil).Header().Get("Location"))
	})

	for _, route := range []string{"/o/acme/projects/1", "/_nuxt", "/_nuxt/missing.js"} {
		t.Run("falls back to index for "+route, func(t *testing.T) {
			t.Parallel()
			rec := get(h, route, nil)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), `baseURL:"/console/"`)
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
		})
	}

	t.Run("missing index returns 404", func(t *testing.T) {
		t.Parallel()
		rec := get(newSPA(fstest.MapFS{}, "/console/", "", ""), "/o/acme", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("unreadable build returns 404", func(t *testing.T) {
		t.Parallel()
		rec := get(newSPA(brokenFS{}, "/console/", "", ""), "/", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestSPA_IndexETagFollowsConfig(t *testing.T) {
	t.Parallel()
	etag := func(apiBase, version string) string {
		return get(newSPA(testFS(), "/console/", apiBase, version), "/", nil).Header().Get("ETag")
	}
	base := etag("/api", "1.2.3")
	assert.Equal(t, base, etag("/api", "1.2.3"))
	assert.NotEqual(t, base, etag("/api", "1.2.4"))
	assert.NotEqual(t, base, etag("/v2/api", "1.2.3"))
}

func TestRebase(t *testing.T) {
	t.Parallel()
	assert.Equal(t, indexHTML, rebase(indexHTML, "/", "/api", "1.2.3"))
	got := rebase(indexHTML, "/oa/console/", "/oa/api", "")
	assert.Contains(t, got, `src="/oa/console/_nuxt/app.js"`)
	assert.Contains(t, got, `baseURL:"/oa/console/"`)
	assert.Contains(t, got, `apiUrl:"/oa/api"`)
	assert.Contains(t, got, `appVersion:""`)
}

func TestHandlerReturnsHandler(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Handler("/console/", "/api", "1.2.3"))
}

type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, errors.New("broken") }
