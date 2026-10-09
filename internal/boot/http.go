package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/web"
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
	return controlplane.New(controlplane.Services{
		Orgs:     svcs.Orgs,
		Projects: svcs.Projects,
		Agents:   svcs.Agents,
	}, unexpected, opts).Handler(basePath)
}

// buildHTTPHandler builds the web surface's handler.
func buildHTTPHandler(
	log *slog.Logger,
	unexpected apperror.UnexpectedFunc,
	ready func() bool,
	basePath string,
	api, consoleHandler, mcpHandler http.Handler,
) http.Handler {
	return web.NewServer(web.ServerOpts{
		APIHandler: api,
		MCPHandler: mcpHandler,
		Console:    consoleHandler,
		BasePath:   basePath,
		Log:        log,
		Unexpected: unexpected,
		Ready:      ready,
	})
}
