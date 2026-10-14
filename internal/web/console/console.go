// Package console embeds the built console SPA.
package console

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed all:public
//nolint:gochecknoglobals // embedded asset dir.
var files embed.FS

// Handler serves the embedded SPA at consoleBase, rebasing Nuxt's baked asset URLs, baseURL, apiUrl and appVersion per request.
func Handler(consoleBase, apiBase, appVersion string) http.Handler {
	sub, err := fs.Sub(files, "public")
	if err != nil {
		return http.NotFoundHandler()
	}
	return &spa{fs: sub, base: consoleBase, apiBase: apiBase, appVersion: appVersion}
}

type spa struct {
	fs         fs.FS
	base       string
	apiBase    string
	appVersion string
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if upath == "." {
		s.serveIndex(w, r)
		return
	}
	if f, err := s.fs.Open(upath); err == nil {
		_ = f.Close()
		http.ServeFileFS(w, r, s.fs, upath) //nolint:gosec // G703: s.fs is the embedded build; fs.FS rejects paths that leave it
		return
	}
	s.serveIndex(w, r)
}

func (s *spa) serveIndex(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(s.fs, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(s.rebase(string(b))))
}

// rebase rewrites Nuxt's baked asset URLs, baseURL, apiUrl and appVersion onto the runtime values.
func (s *spa) rebase(html string) string {
	if s.base == "" || s.base == "/" {
		return html
	}
	html = strings.ReplaceAll(html, `"/_nuxt/`, `"`+s.base+`_nuxt/`)
	html = strings.ReplaceAll(html, `baseURL:"/"`, `baseURL:"`+s.base+`"`)
	if s.apiBase != "" {
		html = strings.ReplaceAll(html, `apiUrl:""`, `apiUrl:"`+s.apiBase+`"`)
	}
	if s.appVersion != "" {
		html = strings.ReplaceAll(html, `appVersion:""`, `appVersion:`+strconv.Quote(s.appVersion))
	}
	return html
}
