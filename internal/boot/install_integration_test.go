//go:build integration

package boot

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/altalune-id/othra/oidcrp/oidcrptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
)

func TestInstallRequired(t *testing.T) {
	svcs, cfg := migratedServices(t, devLocalConfig())
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	assert.True(t, required.Load())

	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	required, err = installRequired(t.Context(), svcs)
	require.NoError(t, err)
	assert.False(t, required.Load())
}

func TestInstallRequired_BasicMode(t *testing.T) {
	cfg := devLocalConfig()
	cfg.Auth = config.AuthConfig{Mode: config.AuthModeBasic, Basic: config.BasicConfig{User: "ops", Password: "s3cret-pass"}}
	svcs, cfg := migratedServices(t, cfg)
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	assert.False(t, required.Load())
}

func TestInstall_EndToEnd(t *testing.T) {
	cfg := devLocalConfig()
	cfg.HTTP.BasePath = "/app"
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	svcs, cfg := migratedServices(t, cfg)
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	svcs.InstallRequired = required
	h := buildHTTPHandler(httpSurfaces{basePath: cfg.HTTP.BasePath, routes: buildRoutes(cfg, svcs, nil, nil), installRequired: required})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/console/", nil))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app/install", rec.Header().Get("Location"))

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/install?token="+url.QueryEscape(svcs.InstallToken.Value()), nil))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	var setup *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "oagent_setup" {
			setup = c
		}
	}
	require.NotNil(t, setup)

	form := url.Values{"name": {"Ada"}, "email": {"Ada@Example.com"}, "password": {"correct horse battery"}, "org_name": {"Acme"}}
	req := httptest.NewRequest(http.MethodPost, "/app/install", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(setup)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/app/console/", rec.Header().Get("Location"))
	assert.False(t, required.Load())

	ctx := t.Context()
	u, err := svcs.Users.ByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	sys, err := svcs.Orgs.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Acme", sys.Name)
	m, err := svcs.Members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	_, err = svcs.Projects.BySlug(ctx, sys.ID, project.DefaultSlug)
	require.NoError(t, err)

	var sid string
	for _, c := range rec.Result().Cookies() {
		if c.Name == session.CookieName {
			sid, err = session.Verify(svcs.StateSecret, c.Value)
			require.NoError(t, err)
		}
	}
	p, ok, err := svcs.Sessions.Load(ctx, sid)
	require.NoError(t, err)
	require.True(t, ok, "a session row exists")
	assert.Equal(t, u.ID, p.UserID)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/install", nil))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app/login", rec.Header().Get("Location"))
}

func TestLogin_EndToEnd(t *testing.T) {
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	svcs.InstallRequired = required

	console := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := session.PrincipalFrom(r.Context())
		_, _ = w.Write([]byte(p.Email + "|" + strings.Join(p.Scopes, ",")))
	})
	h := buildHTTPHandler(httpSurfaces{
		routes:          buildRoutes(cfg, svcs, nil, nil),
		console:         console,
		session:         buildSessionMiddleware(svcs, nil),
		installRequired: required,
	})

	form := url.Values{"email": {"root@example.com"}, "password": {"genesis-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	var sid *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == session.CookieName {
			sid = c
		}
	}
	require.NotNil(t, sid)

	req = httptest.NewRequest(http.MethodGet, "/console/", nil)
	req.AddCookie(sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "root@example.com|*", rec.Body.String())

	req = httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusSeeOther, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/console/", nil)
	req.AddCookie(sid)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "|", rec.Body.String(), "the old cookie no longer signs in")
}

// With passwords off, /install takes only the owner's email; the owner then signs in through the provider.
func TestInstall_EmailOnlyEndToEnd(t *testing.T) {
	idp := oidcrptest.New(t)
	cfg := oidcConfig(idp)
	cfg.Auth.Password.Enabled = new(false)
	svcs, cfg := migratedServices(t, cfg)
	svcs.OIDC = newOIDCProvider(cfg, idp.Client())
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	svcs.InstallRequired = required
	h := buildHTTPHandler(httpSurfaces{routes: buildRoutes(cfg, svcs, nil, nil), session: buildSessionMiddleware(svcs, nil), installRequired: required})

	unlock := serve(h, httptest.NewRequest(http.MethodGet, "/install?token="+url.QueryEscape(svcs.InstallToken.Value()), nil))
	require.Equal(t, http.StatusSeeOther, unlock.Code)
	setup := cookieNamed(unlock, "oagent_setup")
	require.NotNil(t, setup)
	page := serve(h, httptest.NewRequest(http.MethodGet, "/install", nil), setup)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "They sign in with Continue with SSO")
	assert.NotContains(t, page.Body.String(), `name="password"`)

	form := url.Values{"email": {"Ada@Example.com"}, "org_name": {"Acme"}, "password": {"ignored-password"}}
	rec := serve(h, formPost("/install", form), setup)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/login", rec.Header().Get("Location"))
	assert.False(t, required.Load())
	ctx := t.Context()
	_, err = svcs.Users.ByEmail(ctx, "ada@example.com")
	require.Error(t, err, "no account until the owner signs in")
	sys, err := svcs.Orgs.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Acme", sys.Name)

	a := &oidcApp{idp: idp, svcs: svcs, h: h}
	owner := a.signIn(t, oidcrptest.User{Subject: "sub-ada", Email: "ada@example.com", EmailVerified: true, Name: "Ada"})
	require.Equal(t, http.StatusSeeOther, owner.Code, owner.Body.String())
	u, err := svcs.Users.ByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	m, err := svcs.Members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	assert.Equal(t, http.StatusSeeOther, serve(h, httptest.NewRequest(http.MethodGet, "/install", nil)).Code, "/install is closed")
}
