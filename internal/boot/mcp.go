package boot

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/altalune-id/oidcrp"

	"altalune.id/oagent/config"
	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	authv1mcp "altalune.id/oagent/gen/go/auth/v1/authv1mcp"
	invitev1mcp "altalune.id/oagent/gen/go/invite/v1/invitev1mcp"
	memberv1mcp "altalune.id/oagent/gen/go/member/v1/memberv1mcp"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/tokens"
	mcpinternal "altalune.id/oagent/internal/mcp"
	mcpui "altalune.id/oagent/internal/mcp/ui"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/webhook"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/scope"
	"altalune.id/oagent/version"
)

// NOTE: nil when mcp.enabled is off; allowedHosts may arrive on loopback connections through a same-host proxy.
func buildMCPServer(cfg config.MCPConfig, svcs *Services, log *slog.Logger, allowedHosts ...string) *rootmcp.Server {
	if !cfg.Enabled {
		return nil
	}
	registry := rootmcp.NewRegistry()
	refs := newPublicIDs(svcs)
	validate := svcs.Validator.Validate
	orgv1mcp.RegisterOrgServiceTools(registry, org.NewControlPlane(svcs.Orgs), permission.For, validate)
	projectv1mcp.RegisterProjectServiceTools(registry, project.NewControlPlane(svcs.Projects, refs), permission.For, validate)
	agentv1mcp.RegisterAgentServiceTools(registry, agent.NewControlPlane(svcs.Agents, refs), permission.For, validate)
	webhookv1mcp.RegisterWebhookServiceTools(registry, webhook.NewControlPlane(svcs.Webhooks, refs), permission.For, validate)
	memberv1mcp.RegisterMemberServiceTools(registry, member.NewControlPlane(svcs.Members, refs), permission.For, validate)
	invitev1mcp.RegisterInviteServiceTools(registry, invite.NewControlPlane(svcs.Invites, refs), permission.For, validate)
	authv1mcp.RegisterAuthServiceTools(registry, auth.NewControlPlane(svcs.Orgs, svcs.Projects, svcs.Users), permission.For, validate)

	srv := rootmcp.NewServer(
		rootmcp.WithImplementation("oagent", version.Default()),
		rootmcp.WithRegistry(registry),
		rootmcp.WithScopes(mcpinternal.ScopesFromContext),
		rootmcp.WithErrorMapper(mcpinternal.ErrorPayload),
		rootmcp.WithLogger(log),
		rootmcp.WithUI(cfg.AppsUI),
		rootmcp.WithAllowedHosts(allowedHosts...),
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

func mcpPublicHost(c config.HTTPConfig) []string {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	return []string{u.Hostname()}
}

func mcpHTTPHandler(srv *rootmcp.Server) http.Handler {
	if srv == nil {
		return nil
	}
	return srv.Handler()
}

// SECURITY: RFC 8707: the verifier is pinned to the MCP audience, so a token minted for another resource is refused; nil without MCP OAuth.
func buildMCPTokens(cfg config.Config, client *http.Client) (tokens.Verifier, error) {
	if !cfg.MCP.OAuth(cfg.OIDC) {
		return nil, nil
	}
	tc := tokens.Config{Issuer: cfg.OIDC.Issuer, Audience: mcpAudience(cfg), ClockSkew: cfg.MCP.ClockSkew}
	return tokens.NewVerifier(tc, oidcrp.NewKeySource(cfg.OIDC.Issuer, client))
}

func mcpAudience(cfg config.Config) string {
	if cfg.MCP.Audience != "" {
		return cfg.MCP.Audience
	}
	return publicBase(cfg.HTTP) + "/mcp"
}

// NOTE: zero without MCP OAuth; the RFC 9728 document is mounted at the root, outside http.basePath, as RFC 9728 Section 3.1 places it.
func buildMCPResource(cfg config.Config, srv *rootmcp.Server, log *slog.Logger) (tokens.Resource, map[string]http.Handler, error) {
	if srv == nil || !cfg.MCP.OAuth(cfg.OIDC) {
		return tokens.Resource{}, nil, nil
	}
	r, err := tokens.NewResource(mcpAudience(cfg))
	if err != nil {
		return tokens.Resource{}, nil, fmt.Errorf("boot: mcp resource: %w", err)
	}
	log.Info("boot: mcp oauth on", "audience", r.Identifier, "metadata", r.MetadataPath, "issuer", cfg.OIDC.Issuer)
	return r, map[string]http.Handler{r.MetadataPath: r.MetadataHandler(cfg.OIDC.Issuer, registeredScopes(srv.Registry()))}, nil
}

// SECURITY: only catalog scopes a token may hold are offered, so a client never asks consent for one it can't use.
func registeredScopes(reg *rootmcp.Registry) []string {
	out := map[string]struct{}{}
	for _, name := range reg.Names() {
		spec, _ := reg.Spec(name)
		for _, s := range spec.Scopes {
			if scope.Valid(s) && scope.Holds(s, scope.HolderPerson) {
				out[s] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(out))
}

func toolScopes(reg *rootmcp.Registry) func(string) []string {
	return func(name string) []string {
		spec, ok := reg.Spec(name)
		if !ok {
			return nil
		}
		return spec.Scopes
	}
}
