// Package console embeds the built console SPA.
package console

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"altalune.id/oagent/internal/web/assets"
)

//go:embed all:public
//nolint:gochecknoglobals // embedded asset dir.
var files embed.FS

const (
	indexName     = "index.html"
	manifestName  = "_nuxt/builds/latest.json"
	hashedPrefix  = "_nuxt/"
	cacheHashed   = "public, max-age=31536000, immutable"
	cacheShort    = "public, max-age=3600"
	cacheValidate = "no-cache"
)

// Handler serves the embedded SPA at consoleBase, with Nuxt's baked asset URLs, baseURL, apiUrl and appVersion rebased once.
func Handler(consoleBase, apiBase, appVersion string) http.Handler {
	sub, err := fs.Sub(files, "public")
	if err != nil {
		return http.NotFoundHandler()
	}
	return newSPA(sub, consoleBase, apiBase, appVersion)
}

func newSPA(fsys fs.FS, base, apiBase, appVersion string) http.Handler {
	files, err := assets.Load(fsys)
	if err != nil {
		return http.NotFoundHandler()
	}
	s := &spa{files: files}
	if b, err := fs.ReadFile(fsys, indexName); err == nil {
		s.index = assets.New(indexName, []byte(rebase(string(b), base, apiBase, appVersion)))
	}
	return s
}

type spa struct {
	files assets.Files
	index *assets.Asset
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	switch {
	case name == indexName:
		loc := "./"
		if r.URL.RawQuery != "" {
			loc += "?" + r.URL.RawQuery
		}
		w.Header().Set("Location", loc)
		w.WriteHeader(http.StatusMovedPermanently)
	case s.files.Sibling(name):
		http.NotFound(w, r)
	case s.files[name] != nil:
		s.files[name].Serve(w, r, cacheControl(name))
	case s.index != nil:
		s.index.Serve(w, r, cacheValidate)
	default:
		http.NotFound(w, r)
	}
}

func cacheControl(name string) string {
	switch {
	case name == manifestName:
		return cacheValidate
	case strings.HasPrefix(name, hashedPrefix):
		return cacheHashed
	default:
		return cacheShort
	}
}

func rebase(html, base, apiBase, appVersion string) string {
	if base == "" || base == "/" {
		return html
	}
	html = strings.ReplaceAll(html, `"/_nuxt/`, `"`+base+`_nuxt/`)
	html = strings.ReplaceAll(html, `baseURL:"/"`, `baseURL:"`+base+`"`)
	if apiBase != "" {
		html = strings.ReplaceAll(html, `apiUrl:""`, `apiUrl:"`+apiBase+`"`)
	}
	if appVersion != "" {
		html = strings.ReplaceAll(html, `appVersion:""`, `appVersion:`+strconv.Quote(appVersion))
	}
	return html
}
