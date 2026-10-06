package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/auth-server/config"
	orgv1mcp "altalune.id/auth-server/gen/go/org/v1/orgv1mcp"
	mcpinternal "altalune.id/auth-server/internal/mcp"
	mcpui "altalune.id/auth-server/internal/mcp/ui"
	"altalune.id/auth-server/internal/org"
	rootmcp "altalune.id/auth-server/mcp"
	"altalune.id/auth-server/version"
)

// buildMCPServer builds the MCP server, or nil when disabled.
// SECURITY: the scope check is allow-all (mcpinternal.AllScopes) until the authn/tokens port lands.
func buildMCPServer(cfg config.MCPConfig, svcs *Services, log *slog.Logger) *rootmcp.Server {
	if !cfg.Enabled {
		return nil
	}
	registry := rootmcp.NewRegistry()
	orgv1mcp.RegisterOrgServiceTools(registry, org.NewOrgService(svcs.Orgs), mcpinternal.ScopeFor)

	srv := rootmcp.NewServer(
		rootmcp.WithImplementation("auth-server", version.Default()),
		rootmcp.WithRegistry(registry),
		rootmcp.WithScopes(mcpinternal.AllScopes),
		rootmcp.WithErrorMapper(mcpinternal.ErrorPayload),
		rootmcp.WithLogger(log),
		rootmcp.WithUI(cfg.AppsUI),
	)
	if cfg.AppsUI {
		srv.AddUIResource(rootmcp.UIResource{
			URI:           mcpui.ResourceURI,
			Name:          "auth-server app",
			MIMEType:      rootmcp.MIMEApp,
			Body:          mcpui.Document(),
			PrefersBorder: new(bool),
		})
	}
	return srv
}

// mcpHTTPHandler returns srv's streamable-HTTP handler, or nil.
func mcpHTTPHandler(srv *rootmcp.Server) http.Handler {
	if srv == nil {
		return nil
	}
	return srv.Handler()
}
