// Package console embeds the built console SPA.
package console

import (
	"embed"
	"encoding/json"
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

// Handler serves the embedded SPA at consoleBase, with Nuxt's baked asset URLs, baseURL, apiUrl, appVersion and accountUrl rebased once.
func Handler(consoleBase, apiBase, appVersion, accountURL string) http.Handler {
	sub, err := fs.Sub(files, "public")
	if err != nil {
		return http.NotFoundHandler()
	}
	return newSPA(sub, consoleBase, apiBase, appVersion, accountURL)
}

func newSPA(fsys fs.FS, base, apiBase, appVersion, accountURL string) http.Handler {
	files, err := assets.Load(fsys)
	if err != nil {
		return http.NotFoundHandler()
	}
	s := &spa{files: files}
	if b, err := fs.ReadFile(fsys, indexName); err == nil {
		s.index = assets.New(indexName, []byte(rebase(string(b), base, apiBase, appVersion, accountURL)))
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

type swap struct{ old, new string }

// NOTE: every string rebase rewrites lives here; the built-index test checks each old one still appears in Nuxt's output.
func placeholders(base, apiBase, appVersion, accountURL string) []swap {
	out := []swap{
		{`"/_nuxt/`, `"` + base + `_nuxt/`},
		{`baseURL:"/"`, `baseURL:"` + base + `"`},
		{`apiUrl:""`, ""},
		{`appVersion:""`, ""},
		{`accountUrl:""`, ""},
	}
	if apiBase != "" {
		out[2].new = `apiUrl:"` + apiBase + `"`
	}
	if appVersion != "" {
		out[3].new = `appVersion:` + strconv.Quote(appVersion)
	}
	if accountURL != "" {
		// SECURITY: JSON escapes <, > and & so the operator's URL cannot close the inline script.
		quoted, _ := json.Marshal(accountURL)
		out[4].new = `accountUrl:` + string(quoted)
	}
	return out
}

func rebase(html, base, apiBase, appVersion, accountURL string) string {
	if base == "" || base == "/" {
		return html
	}
	for _, p := range placeholders(base, apiBase, appVersion, accountURL) {
		if p.new != "" {
			html = strings.ReplaceAll(html, p.old, p.new)
		}
	}
	return html
}
