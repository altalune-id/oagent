package web

import (
	"embed"
	"io/fs"
	"net/http"
	"sync"

	"altalune.id/oagent/internal/web/assets"
)

//go:embed all:static
var staticFS embed.FS

// StaticFS returns the embedded static assets: the compiled app.css and password.js, the logo and the favicons.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

//nolint:gochecknoglobals // computed once, read-only afterwards.
var loadStatic = sync.OnceValues(func() (assets.Files, error) { return assets.Load(StaticFS()) })

// StaticVersion is the content hash a templ page adds to a /static URL as ?v=; empty for an unknown file.
func StaticVersion(name string) string {
	files, err := loadStatic()
	if err != nil {
		return ""
	}
	return files.Version(name)
}

// NOTE: unversioned URLs revalidate by ETag (no-cache); a matching ?v= is served immutable.
func staticHandler(prefix string, load func() (assets.Files, error)) http.Handler {
	files, err := load()
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix(prefix, files.Handler("no-cache"))
}
