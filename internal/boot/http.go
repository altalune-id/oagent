package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/auth-server/apperror"
	"altalune.id/auth-server/config"
	"altalune.id/auth-server/httpserver"
	"altalune.id/auth-server/internal/controlplane"
	"altalune.id/auth-server/internal/web"
)

// httpConfig maps the operator config onto the HTTP server worker's config.
func httpConfig(c config.HTTPConfig) httpserver.Config {
	return httpserver.Config{Name: "http", Addr: c.Addr}
}

// buildAPIHandler builds the Connect-RPC controlplane handler mounted under BasePath+"/api".
func buildAPIHandler(svcs *Services, unexpected apperror.UnexpectedFunc, cfg config.APIConfig, basePath string) http.Handler {
	if svcs == nil || svcs.Orgs == nil {
		return nil
	}
	opts := controlplane.Options{OpenAPIEnabled: cfg.OpenAPI.Enabled}
	if cfg.OpenAPI.RequireBasicAuth {
		opts.OpenAPIBasicAuth = &controlplane.BasicAuth{User: cfg.OpenAPI.BasicAuthUser, Password: cfg.OpenAPI.BasicAuthPassword}
	}
	return controlplane.New(svcs.Orgs, unexpected, opts).Handler(basePath)
}

// buildHTTPHandler builds the web surface's handler.
func buildHTTPHandler(log *slog.Logger, unexpected apperror.UnexpectedFunc, ready func() bool, basePath string, api, mcpHandler http.Handler) http.Handler {
	return web.NewServer(web.ServerOpts{
		APIHandler: api,
		MCPHandler: mcpHandler,
		BasePath:   basePath,
		Log:        log,
		Unexpected: unexpected,
		Ready:      ready,
	})
}
