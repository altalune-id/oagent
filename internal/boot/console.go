package boot

import (
	"net/http"

	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/console"
)

// buildConsoleHandler builds the console SPA handler mounted at BasePath+"/console".
func buildConsoleHandler(basePath string) http.Handler {
	return console.Handler(
		web.Path(basePath, "/console")+"/",
		web.Path(basePath, "/api"),
	)
}
