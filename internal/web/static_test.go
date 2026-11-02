package web

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/web/assets"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/password"
)

func loader(fsys fs.FS) func() (assets.Files, error) {
	return func() (assets.Files, error) { return assets.Load(fsys) }
}

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

func TestAppCSS_LoadsBundledFonts(t *testing.T) {
	css := string(builtAsset(t, "app.css"))
	urls := regexp.MustCompile(`url\((fonts/[^)]+\.woff2)\)`).FindAllStringSubmatch(css, -1)
	require.Len(t, urls, 4, "Instrument Sans and JetBrains Mono, latin and latin-ext; no font CDN")
	for _, m := range urls {
		_, err := fs.Stat(StaticFS(), m[1])
		assert.NoError(t, err, "app.css loads %s; run make web", m[1])
	}
	for _, family := range []string{"Instrument Sans Variable", "JetBrains Mono Variable"} {
		assert.Contains(t, css, family)
	}
}

func TestAppCSS_FollowsADarkSystem(t *testing.T) {
	css := string(builtAsset(t, "app.css"))
	i := strings.Index(css, "@media (prefers-color-scheme:dark){:root:not([data-theme]){")
	require.GreaterOrEqual(t, i, 0, "sign-in pages set no data-theme, so this rule picks oagent-dark")
	assert.Contains(t, css[i:i+200], "color-scheme:dark")
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
	h := staticHandler("/static/", loader(fstest.MapFS{
		"app.css":    {Data: []byte("body{}")},
		"app.css.br": {Data: []byte("BR")},
		"app.css.gz": {Data: []byte("GZ")},
	}))

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
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"), "an unversioned URL revalidates by ETag")
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

func TestStaticHandler_VersionedURLIsImmutable(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{"app.css": {Data: []byte("body{}")}}
	files, err := assets.Load(fsys)
	require.NoError(t, err)
	h := staticHandler("/static/", loader(fsys))
	for q, want := range map[string]string{
		"?v=" + files.Version("app.css"): "public, max-age=31536000, immutable",
		"?v=stale":                       "no-cache",
		"?v=":                            "no-cache",
		"":                               "no-cache",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.css"+q, nil))
		assert.Equal(t, http.StatusOK, rec.Code, q)
		assert.Equal(t, want, rec.Header().Get("Cache-Control"), q)
	}
}

// The sign-in pages link every static file with its current ?v=, so a deploy never pairs new markup with cached CSS.
func TestSignInPages_LinkVersionedImmutableAssets(t *testing.T) {
	builtAsset(t, "app.css")
	builtAsset(t, "password.js")
	kit := layout.Kit{Version: StaticVersion}
	d := kit.Layout("Sign in")
	field := layout.PasswordField(d, layout.PasswordFieldData{Name: "password", Label: "Password"})
	page := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return layout.Layout(d).Render(templ.WithChildren(ctx, field), w)
	})
	srv := NewServer(ServerOpts{Routes: []Registrar{registrarFunc(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) { kit.Render(w, r, http.StatusOK, page) })
	})}})
	body := get(t, srv, "/login").Body.String()
	urls := regexp.MustCompile(`"(/static/[^"?]+\?v=[0-9a-f]+)"`).FindAllStringSubmatch(body, -1)
	require.GreaterOrEqual(t, len(urls), 2, "app.css and password.js carry ?v=: %s", body)
	for _, m := range urls {
		rec := get(t, srv, m[1])
		assert.Equal(t, http.StatusOK, rec.Code, m[1])
		assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"), m[1])
	}
}

func TestPasswordBundle_ImportsNothing(t *testing.T) {
	assert.NotContains(t, string(builtAsset(t, "password.js")), "import ", "a shared chunk would load unversioned; keep password.js one file")
}

func TestStaticHandler_UnreadableIsNotFound(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	staticHandler("/static/", loader(brokenFS{})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, errors.New("broken") }
