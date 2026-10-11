package boot

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync/atomic"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/web"
	"altalune.id/oagent/internal/web/layout"
	"altalune.id/oagent/internal/web/middleware"
)

// httpConfig maps the operator config onto the HTTP server worker's config.
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
			Permissions:  controlplane.PermissionTable(),
			Logger:       log,
		},
	}
	if cfg.OpenAPI.RequireBasicAuth {
		opts.OpenAPIBasicAuth = &controlplane.BasicAuth{User: cfg.OpenAPI.BasicAuthUser, Password: cfg.OpenAPI.BasicAuthPassword}
	}
	return controlplane.New(controlplane.Services{
		Orgs:     svcs.Orgs,
		Projects: svcs.Projects,
		Agents:   svcs.Agents,
		APIKeys:  svcs.APIKeys,
		Webhooks: svcs.Webhooks,
		Users:    svcs.Users,
	}, unexpected, opts).Handler(basePath)
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

func buildRoutes(cfg config.Config, svcs *Services, log *slog.Logger, unexpected apperror.UnexpectedFunc) []web.Registrar {
	kit := layout.Kit{BasePath: cfg.HTTP.BasePath, Log: log, Unexpected: unexpected}
	legalOpts := legal.WebOpts{Kit: kit, TermsURL: cfg.Compliance.TermsURL, PrivacyURL: cfg.Compliance.PrivacyURL}
	if svcs == nil || !svcs.Caps.LocalLogin {
		return []web.Registrar{legal.NewWeb(legalOpts)}
	}
	sessions := cookieSessions(cfg, svcs)
	if svcs.Users != nil {
		legalOpts.Service = legal.NewService(svcs.Users)
	}
	legalOpts.Sessions = sessions
	legalOpts.RequireAcceptance = cfg.Compliance.RequireAcceptance
	return []web.Registrar{
		legal.NewWeb(legalOpts),
		auth.NewWeb(kit, svcs.Auth, sessions),
		install.NewWeb(install.WebOpts{
			Kit:       kit,
			Installer: svcs.Install,
			SignIn:    svcs.Auth,
			Sessions:  sessions,
			Token:     svcs.InstallToken,
			Required:  svcs.InstallRequired,
			Secret:    svcs.StateSecret,
			Secure:    cfg.HTTP.CookieSecure,
		}),
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

	requireAcceptance bool
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
		SessionGate: legal.WelcomeGate(s.basePath, s.requireAcceptance),
		CrossOrigin: s.crossOrigin,
	})
}
