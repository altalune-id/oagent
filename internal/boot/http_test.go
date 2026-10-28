package boot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web/layout"
)

func TestHTTPConfig(t *testing.T) {
	c := httpConfig(config.HTTPConfig{Addr: ":8080"})
	assert.Equal(t, "http", c.Name)
	assert.Equal(t, ":8080", c.Addr)
}

func TestBuildHTTPHandler_ServesPages(t *testing.T) {
	cfg := config.Config{HTTP: config.HTTPConfig{BasePath: "/app"}}
	h := buildHTTPHandler(httpSurfaces{basePath: "/app", routes: buildRoutes(cfg, nil, nil, nil)})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/terms", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/", nil))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func serveRoutes(cfg config.Config, svcs *Services, method, path string) *httptest.ResponseRecorder {
	h := buildHTTPHandler(httpSurfaces{basePath: cfg.HTTP.BasePath, routes: buildRoutes(cfg, svcs, nil, nil)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestBuildRoutes_TermsOverride(t *testing.T) {
	cfg := config.Config{Compliance: config.ComplianceConfig{TermsURL: "https://example.com/t"}}
	rec := serveRoutes(cfg, nil, http.MethodGet, "/terms")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://example.com/t", rec.Header().Get("Location"))
}

func TestBuildRoutes_WelcomeFollowsFlag(t *testing.T) {
	users := &user.Service{}
	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, Auth: &auth.Service{}, Users: users, Legal: legal.NewService(users)}
	cfg := config.Config{}
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, svcs, http.MethodGet, "/welcome").Code)

	cfg.Compliance.RequireAcceptance = true
	assert.Equal(t, http.StatusSeeOther, serveRoutes(cfg, svcs, http.MethodGet, "/welcome").Code, "anonymous goes to /login")
}

func TestBuildRoutes_LoginOnlyWithLocalLogin(t *testing.T) {
	cfg := config.Config{}
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodGet, "/login").Code)
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodPost, "/logout").Code)

	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodGet, "/forgot-password").Code)

	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true, Passwords: true}, Auth: &auth.Service{}, Resets: auth.NewResets(auth.ResetDeps{})}
	assert.Equal(t, http.StatusOK, serveRoutes(cfg, svcs, http.MethodGet, "/login").Code)
	assert.Contains(t, serveRoutes(cfg, svcs, http.MethodGet, "/login").Body.String(), "Forgot password?")
	assert.Equal(t, http.StatusOK, serveRoutes(cfg, svcs, http.MethodGet, "/forgot-password").Code)
}

// auth.password.enabled off: no password sign-in, no reset pages, and the invite landing offers no password join.
func TestBuildRoutes_PasswordsOff(t *testing.T) {
	cfg := config.Config{}
	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, Auth: &auth.Service{}, Resets: auth.NewResets(auth.ResetDeps{})}
	login := serveRoutes(cfg, svcs, http.MethodGet, "/login")
	assert.Equal(t, http.StatusOK, login.Code)
	assert.NotContains(t, login.Body.String(), `name="password"`)
	assert.Equal(t, http.StatusMethodNotAllowed, serveRoutes(cfg, svcs, http.MethodPost, "/login").Code)
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, svcs, http.MethodGet, "/forgot-password").Code)
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, svcs, http.MethodGet, "/reset-password/abc").Code)
	assert.Empty(t, inviteMethods(svcs))
	assert.Equal(t, []string{invite.MethodPassword}, inviteMethods(&Services{Caps: capabilities.Capabilities{LocalLogin: true, Passwords: true}}))
	assert.Equal(t, []string{invite.MethodPassword, invite.MethodOIDC}, inviteMethods(&Services{
		Caps: capabilities.Capabilities{LocalLogin: true, Passwords: true}, OIDC: newOIDCProvider(config.Config{}, nil),
	}))
}

func TestAPIChain(t *testing.T) {
	local := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, KeyAuthn: &apikey.Authenticator{}, Auth: &auth.Service{}}
	chain := apiChain(local)
	require.Len(t, chain, 2)
	assert.IsType(t, &apikey.Authenticator{}, chain[0])
	assert.IsType(t, auth.SessionAuthenticator{}, chain[1])

	basic := &Services{Caps: capabilities.Capabilities{BasicAuth: true}, KeyAuthn: &apikey.Authenticator{}, Auth: &auth.Service{}}
	chain = apiChain(basic)
	require.Len(t, chain, 2)
	assert.IsType(t, auth.BasicAuthenticator{}, chain[1])
}

func TestBuildCrossOrigin(t *testing.T) {
	cop, err := buildCrossOrigin("https://oagent.example.com/app")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
	req.Host = "internal:8080"
	req.Header.Set("Origin", "https://oagent.example.com")
	assert.NoError(t, cop.Check(req))
	req.Header.Set("Origin", "https://evil.example")
	assert.Error(t, cop.Check(req))

	cop, err = buildCrossOrigin("")
	require.NoError(t, err)
	assert.NotNil(t, cop)
}

func TestWarnSampleLegal(t *testing.T) {
	prod := config.Config{Env: config.EnvProduction}
	set := prod
	set.Compliance = config.ComplianceConfig{TermsURL: "https://example.com/terms", PrivacyURL: "https://example.com/privacy"}
	dev := config.Config{Env: config.EnvDevelopment}

	tests := []struct {
		name    string
		cfg     config.Config
		serving bool
		want    string
	}{
		{name: "production, both empty", cfg: prod, serving: true, want: "missing=\"compliance.termsURL compliance.privacyURL\""},
		{name: "production, both set", cfg: set, serving: true},
		{name: "development", cfg: dev, serving: true},
		{name: "not serving http (mcp stdio, set-password)", cfg: prod, serving: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, buf := bufferLogger()
			warnSampleLegal(tt.cfg, log, tt.serving)
			if tt.want == "" {
				assert.Empty(t, buf.String())
				return
			}
			assert.Equal(t, 1, strings.Count(buf.String(), "level=WARN"))
			assert.Contains(t, buf.String(), "boot: serving the built-in sample terms/privacy; set compliance.termsURL and compliance.privacyURL")
			assert.Contains(t, buf.String(), tt.want)
		})
	}
}

func TestWarnSampleLegal_NamesOnlyTheMissingKey(t *testing.T) {
	cfg := config.Config{Env: config.EnvProduction, Compliance: config.ComplianceConfig{TermsURL: "https://example.com/terms"}}
	log, buf := bufferLogger()
	warnSampleLegal(cfg, log, true)
	assert.Contains(t, buf.String(), "missing=compliance.privacyURL")
}

func TestBuildLegal_TermsDate(t *testing.T) {
	cfg := config.Config{}
	builtIn := buildLegal(kernel.Kernel{}, cfg, nil).TermsUpdatedAt()
	assert.False(t, builtIn.IsZero(), "the built-in documents carry a date")

	cfg.Compliance.TermsURL = "https://legal.example.com/terms"
	cfg.Compliance.TermsUpdatedAt = "2027-01-02"
	assert.Equal(t, time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), buildLegal(kernel.Kernel{}, cfg, nil).TermsUpdatedAt())
}

func TestConsentData(t *testing.T) {
	cfg := config.Config{}
	cfg.Compliance.RequireAcceptance = true
	cfg.Compliance.PrivacyURL = "https://legal.example.com/privacy"
	assert.Equal(t, layout.ConsentData{PrivacyURL: "https://legal.example.com/privacy", Required: true}, consentData(cfg))
}

// SECURITY: a cached pre-sign-in page replays its one-time verdict; a token page's Referer leaks its token.
func TestBuildRoutes_PreSignInPagesNoStore(t *testing.T) {
	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true, Passwords: true}, Auth: &auth.Service{}, Resets: auth.NewResets(auth.ResetDeps{})}
	cfg := config.Config{}
	for _, tc := range []struct {
		method, path, referrer string
	}{
		{http.MethodGet, "/login", "strict-origin-when-cross-origin"},
		{http.MethodPost, "/login", "strict-origin-when-cross-origin"},
		{http.MethodGet, "/install", "strict-origin-when-cross-origin"},
		{http.MethodGet, "/invite", "no-referrer"},
		{http.MethodGet, "/invite/abc", "no-referrer"},
		{http.MethodGet, "/forgot-password", "strict-origin-when-cross-origin"},
		{http.MethodGet, "/reset-password/abc", "no-referrer"},
		{http.MethodGet, "/terms", "strict-origin-when-cross-origin"},
		{http.MethodGet, "/privacy", "strict-origin-when-cross-origin"},
		{http.MethodGet, "/nope", "strict-origin-when-cross-origin"},
	} {
		rec := serveRoutes(cfg, svcs, tc.method, tc.path)
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"), tc.method+" "+tc.path)
		assert.Equal(t, tc.referrer, rec.Header().Get("Referrer-Policy"), tc.method+" "+tc.path)
	}
}

func TestBuildRoutes_InviteOnlyWithLocalLogin(t *testing.T) {
	cfg := config.Config{}
	assert.Equal(t, http.StatusNotFound, serveRoutes(cfg, nil, http.MethodGet, "/invite").Code)
	svcs := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, Auth: &auth.Service{}}
	assert.Equal(t, http.StatusBadRequest, serveRoutes(cfg, svcs, http.MethodGet, "/invite").Code, "no invite cookie")
}

func TestBuildLegal_AnyCustomURLKeepsNoHistory(t *testing.T) {
	cfg := config.Config{}
	cfg.Compliance.PrivacyURL = "https://legal.example.com/privacy"
	l := buildLegal(kernel.Kernel{}, cfg, nil)
	require.NoError(t, l.Publish(t.Context()), "nothing is snapshotted, so no database is touched")
	assert.True(t, l.TermsUpdatedAt().IsZero(), "custom terms are re-asked only by compliance.termsUpdatedAt")
}
