package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
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
		APIKeys:  svcs.APIKeys,
		Webhooks: svcs.Webhooks,
	}, unexpected, opts).Handler(basePath)
}

func buildRoutes(cfg config.Config, log *slog.Logger, unexpected apperror.UnexpectedFunc) []web.Registrar {
	kit := layout.Kit{BasePath: cfg.HTTP.BasePath, Log: log, Unexpected: unexpected}
	return []web.Registrar{
		legal.NewWeb(legal.WebOpts{Kit: kit, TermsURL: cfg.Compliance.TermsURL, PrivacyURL: cfg.Compliance.PrivacyURL}),
	}
}

type httpSurfaces struct {
	log        *slog.Logger
	unexpected apperror.UnexpectedFunc
	ready      func() bool
	basePath   string
	api        http.Handler
	console    http.Handler
	mcp        http.Handler
	routes     []web.Registrar
}

func buildHTTPHandler(s httpSurfaces) http.Handler {
	return web.NewServer(web.ServerOpts{
		APIHandler: s.api,
		MCPHandler: s.mcp,
		Console:    s.console,
		Routes:     s.routes,
		BasePath:   s.basePath,
		Log:        s.log,
		Unexpected: s.unexpected,
		Ready:      s.ready,
	})
}
