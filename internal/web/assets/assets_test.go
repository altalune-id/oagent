package assets

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const plainJS = "console.log('plain')"

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"_nuxt/app.js":    {Data: []byte(plainJS)},
		"_nuxt/app.js.br": {Data: []byte("BROTLI")},
		"_nuxt/app.js.gz": {Data: []byte("GZIP")},
		"logo.svg":        {Data: []byte("<svg/>")},
		"data.nope":       {Data: []byte("raw")},
		"data.nope.gz":    {Data: []byte("raw-gz")},
	}
}

func load(t *testing.T, fsys fs.FS) Files {
	t.Helper()
	files, err := Load(fsys)
	require.NoError(t, err)
	return files
}

func serve(t *testing.T, a *Asset, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	a.Serve(rec, req, "public, max-age=60")
	return rec
}

func TestServe_Encodings(t *testing.T) {
	t.Parallel()
	files := load(t, testFS())
	js := files["_nuxt/app.js"]
	require.NotNil(t, js)

	tests := []struct {
		name     string
		accept   string
		wantBody string
		wantEnc  string
	}{
		{"no accept-encoding gets plain bytes", "", plainJS, ""},
		{"brotli preferred", "gzip, deflate, br", "BROTLI", "br"},
		{"gzip only gets gzip", "gzip", "GZIP", "gzip"},
		{"refused brotli falls to gzip", "br;q=0, gzip", "GZIP", "gzip"},
		{"wildcard takes brotli", "*", "BROTLI", "br"},
		{"wildcard refused but gzip named", "*;q=0, gzip;q=0.5", "GZIP", "gzip"},
		{"identity only gets plain", "identity", plainJS, ""},
		{"bad qvalue is a refusal", "br;q=x, gzip;q=0", plainJS, ""},
		{"case and spaces ignored", " GZIP ; Q=1 ", "GZIP", "gzip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			if tt.accept != "" {
				h.Set("Accept-Encoding", tt.accept)
			}
			rec := serve(t, js, h)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
			assert.Equal(t, tt.wantEnc, rec.Header().Get("Content-Encoding"))
			assert.Equal(t, "text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
			assert.Equal(t, "public, max-age=60", rec.Header().Get("Cache-Control"))
			assert.NotEmpty(t, rec.Header().Get("ETag"))
			assert.Equal(t, strconv.Itoa(len(tt.wantBody)), rec.Header().Get("Content-Length"))
		})
	}
}

func TestServe_ETagPerEncodingAndNotModified(t *testing.T) {
	t.Parallel()
	js := load(t, testFS())["_nuxt/app.js"]

	plain := serve(t, js, nil).Header().Get("ETag")
	br := serve(t, js, http.Header{"Accept-Encoding": {"br"}}).Header().Get("ETag")
	gz := serve(t, js, http.Header{"Accept-Encoding": {"gzip"}}).Header().Get("ETag")
	assert.Regexp(t, `^"[0-9a-f]{32}"$`, plain)
	assert.NotEqual(t, plain, br)
	assert.NotEqual(t, plain, gz)
	assert.NotEqual(t, br, gz)

	rec := serve(t, js, http.Header{"If-None-Match": {plain}})
	assert.Equal(t, http.StatusNotModified, rec.Code)
	assert.Empty(t, rec.Body.String())

	rec = serve(t, js, http.Header{"Accept-Encoding": {"br"}, "If-None-Match": {br}})
	assert.Equal(t, http.StatusNotModified, rec.Code)

	rec = serve(t, js, http.Header{"Accept-Encoding": {"gzip"}, "If-None-Match": {br}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "GZIP", rec.Body.String())
}

func TestServe_RangeGetsPlainBytes(t *testing.T) {
	t.Parallel()
	js := load(t, testFS())["_nuxt/app.js"]
	rec := serve(t, js, http.Header{"Accept-Encoding": {"br, gzip"}, "Range": {"bytes=0-6"}})
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, plainJS[:7], rec.Body.String())
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
}

func TestServe_HeadSendsHeadersOnly(t *testing.T) {
	t.Parallel()
	js := load(t, testFS())["_nuxt/app.js"]
	req := httptest.NewRequest(http.MethodHead, "/x", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	js.Serve(rec, req, "no-cache")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "br", rec.Header().Get("Content-Encoding"))
}

func TestLoad(t *testing.T) {
	t.Parallel()
	files := load(t, testFS())

	t.Run("compressed siblings are variants, not files", func(t *testing.T) {
		t.Parallel()
		assert.NotContains(t, files, "_nuxt/app.js.br")
		assert.NotContains(t, files, "_nuxt/app.js.gz")
		assert.NotContains(t, files, "data.nope.gz")
	})

	t.Run("a file without siblings has no Vary", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, files["logo.svg"], http.Header{"Accept-Encoding": {"br, gzip"}})
		assert.Equal(t, "<svg/>", rec.Body.String())
		assert.Equal(t, "image/svg+xml", rec.Header().Get("Content-Type"))
		assert.Empty(t, rec.Header().Get("Vary"))
	})

	t.Run("a file with no known type is served plain only", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, files["data.nope"], http.Header{"Accept-Encoding": {"gzip"}})
		assert.Equal(t, "raw", rec.Body.String())
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
	})

	t.Run("the ETag follows the content", func(t *testing.T) {
		t.Parallel()
		changed := testFS()
		changed["_nuxt/app.js"] = &fstest.MapFile{Data: []byte(plainJS + ";")}
		before := serve(t, files["_nuxt/app.js"], nil).Header().Get("ETag")
		after := serve(t, load(t, changed)["_nuxt/app.js"], nil).Header().Get("ETag")
		assert.NotEqual(t, before, after)
	})

	t.Run("a broken FS is an error", func(t *testing.T) {
		t.Parallel()
		_, err := Load(brokenFS{})
		require.Error(t, err)
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("over 1 KiB gets an in-memory gzip variant", func(t *testing.T) {
		t.Parallel()
		html := "<html>" + strings.Repeat("<p>hello</p>", 200) + "</html>"
		a := New("index.html", []byte(html))
		rec := serve(t, a, http.Header{"Accept-Encoding": {"gzip, br"}})
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		got, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.Equal(t, html, string(got))
	})

	t.Run("small content stays plain", func(t *testing.T) {
		t.Parallel()
		rec := serve(t, New("index.html", []byte("<html></html>")), http.Header{"Accept-Encoding": {"gzip"}})
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.Empty(t, rec.Header().Get("Vary"))
	})

	t.Run("content that does not shrink stays plain", func(t *testing.T) {
		t.Parallel()
		noise := make([]byte, 4096)
		_, _ = rand.Read(noise)
		rec := serve(t, New("index.html", noise), http.Header{"Accept-Encoding": {"gzip"}})
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
		assert.True(t, bytes.Equal(noise, rec.Body.Bytes()))
	})
}

func TestFilesHandler(t *testing.T) {
	t.Parallel()
	h := load(t, testFS()).Handler("public, max-age=3600")

	tests := []struct {
		path string
		want int
	}{
		{"/_nuxt/app.js", http.StatusOK},
		{"/_nuxt/app.js.br", http.StatusNotFound},
		{"/_nuxt/app.js.gz", http.StatusNotFound},
		{"/_nuxt", http.StatusNotFound},
		{"/missing.js", http.StatusNotFound},
		{"/../logo.svg", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			assert.Equal(t, tt.want, rec.Code)
			if tt.want == http.StatusOK {
				assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestFilesSibling(t *testing.T) {
	t.Parallel()
	files := load(t, testFS())
	assert.True(t, files.Sibling("_nuxt/app.js.br"))
	assert.True(t, files.Sibling("_nuxt/app.js.gz"))
	assert.False(t, files.Sibling("_nuxt/app.js"))
	assert.False(t, files.Sibling("other.js.br"))
}

type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, errors.New("broken") }
