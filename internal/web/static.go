package web

import (
	"embed"
	"io/fs"
	"net/http"

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

func staticHandler(prefix string, fsys fs.FS) http.Handler {
	files, err := assets.Load(fsys)
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix(prefix, files.Handler("public, max-age=3600"))
}
