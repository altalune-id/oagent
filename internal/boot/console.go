package boot

import (
	"net/http"

	"altalune.id/oagent/internal/web/console"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/version"
)

func buildConsoleHandler(basePath string) http.Handler {
	return console.Handler(
		layout.Path(basePath, "/console")+"/",
		layout.Path(basePath, "/api"),
		version.Default(),
	)
}
