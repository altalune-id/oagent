package web

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/password"
)

func TestPasswordBundle_IsBuiltFromTheCurrentRules(t *testing.T) {
	bundle := string(builtAsset(t, "password.js"))

	for name, want := range map[string]int{"minLength": password.MinLen, "maxLength": password.MaxLen} {
		re := regexp.MustCompile(`\b` + name + `:\s*` + strconv.Itoa(want) + `\b`)
		assert.Regexp(t, re, bundle, "password.js is stale; run make web-js")
	}

	raw, err := os.ReadFile("../../password/common.txt")
	require.NoError(t, err)
	n := 0
	for line := range strings.Lines(string(raw)) {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		n++
		// The bundler inlines the list as a template literal (real newlines) or a string literal (escaped \n).
		if !strings.Contains(bundle, "\n"+entry+"\n") && !strings.Contains(bundle, `\n`+entry+`\n`) {
			t.Fatalf("password.js has no %q from password/common.txt; run make web-js", entry)
		}
	}
	assert.Positive(t, n)
}

// NOTE: make web builds these files (not committed); missing, the test skips locally and fails when CI is set.
func builtAsset(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fs.ReadFile(StaticFS(), name)
	if errors.Is(err, fs.ErrNotExist) && os.Getenv("CI") == "" {
		t.Skipf("internal/web/static/%s is not built; run make web", name)
	}
	require.NoError(t, err)
	return b
}

func TestStaticHandler(t *testing.T) {
	t.Parallel()
	h := staticHandler("/static/", fstest.MapFS{
		"app.css":    {Data: []byte("body{}")},
		"app.css.br": {Data: []byte("BR")},
		"app.css.gz": {Data: []byte("GZ")},
	})

	tests := []struct {
		name     string
		path     string
		accept   string
		want     int
		wantBody string
		wantEnc  string
	}{
		{"plain without accept-encoding", "/static/app.css", "", http.StatusOK, "body{}", ""},
		{"brotli", "/static/app.css", "gzip, br", http.StatusOK, "BR", "br"},
		{"gzip only", "/static/app.css", "gzip", http.StatusOK, "GZ", "gzip"},
		{"sibling by name", "/static/app.css.br", "", http.StatusNotFound, "", ""},
		{"directory", "/static/", "", http.StatusNotFound, "", ""},
		{"missing", "/static/nope.js", "", http.StatusNotFound, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept-Encoding", tt.accept)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
			if tt.want != http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantBody, rec.Body.String())
			assert.Equal(t, tt.wantEnc, rec.Header().Get("Content-Encoding"))
			assert.Equal(t, "text/css; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
			assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
			etag := rec.Header().Get("ETag")
			require.NotEmpty(t, etag)

			req.Header.Set("If-None-Match", etag)
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusNotModified, rec.Code)
		})
	}
}

func TestStaticHandler_UnreadableIsNotFound(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	staticHandler("/static/", brokenFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, errors.New("broken") }
