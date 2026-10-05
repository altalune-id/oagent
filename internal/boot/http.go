package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/auth-server/apperror"
	"altalune.id/auth-server/config"
	"altalune.id/auth-server/httpserver"
	"altalune.id/auth-server/internal/web"
)

// httpConfig maps the operator config onto the HTTP server worker's config.
func httpConfig(c config.HTTPConfig) httpserver.Config {
	return httpserver.Config{Name: "http", Addr: c.Addr}
}

// buildHTTPHandler builds the web surface's handler.
func buildHTTPHandler(log *slog.Logger, unexpected apperror.UnexpectedFunc, ready func() bool) http.Handler {
	return web.NewServer(web.ServerOpts{Log: log, Unexpected: unexpected, Ready: ready})
}
