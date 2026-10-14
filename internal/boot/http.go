package boot

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal"
	mcpinternal "altalune.id/oagent/internal/mcp"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
	rootmcp "altalune.id/oagent/mcp"
)

// SECURITY: the lockdown switch; /api, /mcp and the console refuse a request without a credential. Revert its commit to undo.
const requireCredentials = true

func httpConfig(c config.HTTPConfig) httpserver.Config {
	return httpserver.Config{Name: "http", Addr: c.Addr}
}

func buildAPIHandler(
	svcs *Services,
	unexpected apperror.UnexpectedFunc,
	cfg config.APIConfig,
	basePath string,
	log *slog.Logger,
) http.Handler {
	if svcs == nil || svcs.Orgs == nil {
		return nil
	}
	opts := controlplane.Options{
		OpenAPIEnabled: cfg.OpenAPI.Enabled,
		Authn: authn.InterceptorOptions{
			Chain:        apiChain(svcs),
			KeyPrefix:    svcs.KeyScheme.Prefix(),
			CookieSecret: svcs.StateSecret,
			Permissions:  permission.Table(),
			Required:     requireCredentials,
			Logger:       log,
		},
		Validator: svcs.Validator,
	}
	if cfg.OpenAPI.RequireBasicAuth {
		opts.OpenAPIBasicAuth = &controlplane.BasicAuth{User: cfg.OpenAPI.BasicAuthUser, Password: cfg.OpenAPI.BasicAuthPassword}
	}
	h := controlplane.New(controlplane.Services{
		Orgs:     svcs.Orgs,
		Projects: svcs.Projects,
		Agents:   svcs.Agents,
		APIKeys:  svcs.APIKeys,
		Webhooks: svcs.Webhooks,
		Users:    svcs.Users,
		Members:  svcs.Members,
		Invites:  svcs.Invites,
		Auth:     svcs.Auth,
		Refs:     newPublicIDs(svcs),
	}, unexpected, opts).Handler(basePath)
	if requireCredentials && svcs.Caps.BasicAuth {
		h = auth.BasicChallenge(h)
	}
	return h
}

// NOTE: nil before the lockdown; local mode sends a signed-out caller to /login, basic mode challenges.
func buildConsoleGate(cfg config.Config, svcs *Services) func(http.Handler) http.Handler {
	if !requireCredentials || svcs == nil {
		return nil
	}
	if svcs.Caps.BasicAuth {
		return auth.ConsoleGate(cfg.HTTP.BasePath, svcs.Auth)
	}
	return auth.ConsoleGate(cfg.HTTP.BasePath, nil)
}

func apiChain(svcs *Services) authn.Chain {
	chain := authn.Chain{svcs.KeyAuthn}
	if svcs.Caps.LocalLogin {
		chain = append(chain, auth.SessionAuthenticator{Sessions: svcs.Sessions, Auth: svcs.Auth})
	}
	if svcs.Caps.BasicAuth {
		chain = append(chain, auth.BasicAuthenticator{Auth: svcs.Auth})
	}
	return chain
}

func buildMCPHandler(srv *rootmcp.Server, svcs *Services, log *slog.Logger) http.Handler {
	h := mcpHTTPHandler(srv)
	if h == nil {
		return nil
	}
	return mcpinternal.Authenticate(mcpinternal.AuthOptions{
		Chain:     mcpChain(svcs),
		KeyPrefix: svcs.KeyScheme.Prefix(),
		Required:  requireCredentials,
		Logger:    log,
	})(h)
}

// SECURITY: no session authenticator — /mcp has no CSRF guard, so a browser cookie must never act on it.
func mcpChain(svcs *Services) authn.Chain {
	chain := authn.Chain{svcs.KeyAuthn}
	if svcs.Caps.BasicAuth {
		chain = append(chain, auth.BasicAuthenticator{Auth: svcs.Auth})
	}
	return chain
}

// SECURITY: behind a proxy the Host header is not the public origin, so the configured baseURL origin is trusted explicitly.
func buildCrossOrigin(baseURL string) (*http.CrossOriginProtection, error) {
	cop := http.NewCrossOriginProtection()
	if baseURL == "" {
		return cop, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("boot: http.baseURL: %w", err)
	}
	if err := cop.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
		return nil, fmt.Errorf("boot: http.baseURL origin: %w", err)
	}
	return cop, nil
}

func warnSampleLegal(cfg config.Config, log *slog.Logger, serving bool) {
	if log == nil || !serving || cfg.Env == config.EnvDevelopment {
		return
	}
	var missing []string
	if cfg.Compliance.TermsURL == "" {
		missing = append(missing, "compliance.termsURL")
	}
	if cfg.Compliance.PrivacyURL == "" {
		missing = append(missing, "compliance.privacyURL")
	}
	if len(missing) == 0 {
		return
	}
	log.Warn("boot: serving the built-in sample terms/privacy; set compliance.termsURL and compliance.privacyURL",
		slog.String("missing", strings.Join(missing, " ")))
}

func buildRoutes(cfg config.Config, svcs *Services, log *slog.Logger, unexpected apperror.UnexpectedFunc) []web.Registrar {
	kit := layout.Kit{BasePath: cfg.HTTP.BasePath, Log: log, Unexpected: unexpected}
	legalOpts := legal.WebOpts{Kit: kit, TermsURL: cfg.Compliance.TermsURL, PrivacyURL: cfg.Compliance.PrivacyURL}
	if svcs == nil || !svcs.Caps.LocalLogin {
		return []web.Registrar{legal.NewWeb(legalOpts)}
	}
	sessions := cookieSessions(cfg, svcs)
	limits := authLimits(cfg, svcs)
	legalOpts.Service = svcs.Legal
	legalOpts.Sessions = sessions
	legalOpts.RequireAcceptance = cfg.Compliance.RequireAcceptance
	return []web.Registrar{
		legal.NewWeb(legalOpts),
		auth.NewWeb(kit, svcs.Auth, sessions, newPublicIDs(svcs)).WithLimits(limits).WithResets(svcs.Resets),
		install.NewWeb(install.WebOpts{
			Kit:       kit,
			Installer: svcs.Install,
			SignIn:    svcs.Auth,
			Sessions:  sessions,
			Token:     svcs.InstallToken,
			Required:  svcs.InstallRequired,
			Secret:    svcs.StateSecret,
			Secure:    cfg.HTTP.CookieSecure,
			Consent:   consentData(cfg),
		}),
		invite.NewWeb(invite.WebOpts{
			Kit:          kit,
			Invites:      svcs.Invites,
			Users:        svcs.Users,
			Members:      svcs.Members,
			Orgs:         memberOrgs{orgs: svcs.Orgs},
			OrgLinks:     memberOrgs{orgs: svcs.Orgs},
			People:       memberPeople{users: svcs.Users},
			SignIn:       svcs.Auth,
			Sessions:     sessions,
			Consent:      consentData(cfg),
			Methods:      []string{invite.MethodPassword},
			StateSecret:  svcs.StateSecret,
			CookieSecure: cfg.HTTP.CookieSecure,
			Limits:       limits,
		}),
	}
}

func authLimits(cfg config.Config, svcs *Services) auth.Limits {
	return auth.Limits{Limiter: svcs.AuthLimiter, TrustedProxyHops: cfg.HTTP.TrustedProxyHops}
}

func consentData(cfg config.Config) layout.ConsentData {
	return layout.ConsentData{
		TermsURL:   cfg.Compliance.TermsURL,
		PrivacyURL: cfg.Compliance.PrivacyURL,
		Required:   cfg.Compliance.RequireAcceptance,
	}
}

func cookieSessions(cfg config.Config, svcs *Services) auth.CookieSessions {
	return auth.CookieSessions{
		Store:  svcs.Sessions,
		Secret: svcs.StateSecret,
		Cookie: session.CookieOptions{Path: layout.Path(cfg.HTTP.BasePath, ""), Secure: cfg.HTTP.CookieSecure},
		TTL:    cfg.Auth.SessionTTL,
	}
}

func buildSessionMiddleware(svcs *Services, unexpected apperror.UnexpectedFunc) func(http.Handler) http.Handler {
	if svcs == nil || !svcs.Caps.LocalLogin {
		return nil
	}
	return middleware.Session(middleware.SessionConfig{
		Store:      svcs.Sessions,
		Secret:     svcs.StateSecret,
		Auth:       svcs.Auth,
		Unexpected: unexpected,
	})
}

type httpSurfaces struct {
	log        *slog.Logger
	unexpected apperror.UnexpectedFunc
	probes     web.Registrar
	basePath   string
	api        http.Handler
	console    http.Handler
	mcp        http.Handler
	routes     []web.Registrar

	installRequired *atomic.Bool
	session         func(http.Handler) http.Handler
	consoleGate     func(http.Handler) http.Handler

	requireAcceptance bool
	termsUpdated      time.Time
	crossOrigin       *http.CrossOriginProtection
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
		Probes:     s.probes,

		Gate:        install.InstallGate(s.basePath, s.installRequired),
		Session:     s.session,
		SessionGate: legal.WelcomeGate(s.basePath, s.requireAcceptance, s.termsUpdated),
		ConsoleGate: s.consoleGate,
		CrossOrigin: s.crossOrigin,
	})
}
