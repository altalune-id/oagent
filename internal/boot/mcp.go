package boot

import (
	"log/slog"
	"net/http"

	"altalune.id/oagent/config"
	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	authv1mcp "altalune.id/oagent/gen/go/auth/v1/authv1mcp"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/auth"
	mcpinternal "altalune.id/oagent/internal/mcp"
	mcpui "altalune.id/oagent/internal/mcp/ui"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/webhook"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/version"
)

// NOTE: nil when mcp.enabled is off.
func buildMCPServer(cfg config.MCPConfig, svcs *Services, log *slog.Logger) *rootmcp.Server {
	if !cfg.Enabled {
		return nil
	}
	registry := rootmcp.NewRegistry()
	orgv1mcp.RegisterOrgServiceTools(registry, org.NewControlPlane(svcs.Orgs), permission.For)
	projectv1mcp.RegisterProjectServiceTools(registry, project.NewControlPlane(svcs.Projects), permission.For)
	agentv1mcp.RegisterAgentServiceTools(registry, agent.NewControlPlane(svcs.Agents), permission.For)
	webhookv1mcp.RegisterWebhookServiceTools(registry, webhook.NewControlPlane(svcs.Webhooks), permission.For)
	authv1mcp.RegisterAuthServiceTools(registry, auth.NewControlPlane(svcs.Orgs, svcs.Users), permission.For)

	srv := rootmcp.NewServer(
		rootmcp.WithImplementation("oagent", version.Default()),
		rootmcp.WithRegistry(registry),
		rootmcp.WithScopes(mcpinternal.ScopesFromContext),
		rootmcp.WithErrorMapper(mcpinternal.ErrorPayload),
		rootmcp.WithLogger(log),
		rootmcp.WithUI(cfg.AppsUI),
	)
	if cfg.AppsUI {
		srv.AddUIResource(rootmcp.UIResource{
			URI:           mcpui.ResourceURI,
			Name:          "oagent app",
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
