//go:build integration

package boot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altalune-id/othra/oidcrp/oidcrptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/session"
)

type oidcApp struct {
	idp  *oidcrptest.IDP
	svcs *Services
	h    http.Handler
}

func newOIDCApp(t *testing.T, passwords bool) *oidcApp {
	t.Helper()
	idp := oidcrptest.New(t)
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	cfg.OIDC = config.OIDCConfig{Issuer: idp.Issuer(), ClientID: "oagent", Scopes: []string{"openid", "profile", "email"}, ButtonLabel: "Continue with SSO"}
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	svcs.Caps.Passwords = passwords
	svcs.OIDC = newOIDCProvider(cfg, idp.Client())
	h := buildHTTPHandler(httpSurfaces{
		api:    buildAPIHandler(svcs, nil, cfg.API, "", testLogger()),
		routes: buildRoutes(cfg, svcs, nil, nil), session: buildSessionMiddleware(svcs, nil),
	})
	return &oidcApp{idp: idp, svcs: svcs, h: h}
}

func (a *oidcApp) signIn(t *testing.T, user oidcrptest.User) *httptest.ResponseRecorder {
	t.Helper()
	a.idp.User = user
	start := serve(a.h, httptest.NewRequest(http.MethodGet, "/login/oidc?return_to=/console/", nil))
	require.Equal(t, http.StatusFound, start.Code, start.Body.String())
	q := a.idp.Authorize(start.Header().Get("Location"))
	return serve(a.h, httptest.NewRequest(http.MethodGet, "/oauth/callback?"+q.Encode(), nil), cookieNamed(start, "oagent_signin"))
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// SECURITY: a verified email links the password account once; after that the sub alone signs it in.
func TestOIDCSignIn_EndToEnd(t *testing.T) {
	a := newOIDCApp(t, true)
	ctx := t.Context()

	login := serve(a.h, httptest.NewRequest(http.MethodGet, "/login", nil))
	assert.Contains(t, login.Body.String(), "Continue with SSO")
	assert.Contains(t, login.Body.String(), `name="password"`)

	rec := a.signIn(t, oidcrptest.User{Subject: "sub-root", Email: "Root@Example.com", EmailVerified: true, Name: "Root"})
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/", rec.Header().Get("Location"))
	sid := cookieNamed(rec, session.CookieName)
	require.NotNil(t, sid)
	who := serve(a.h, whoamiRequest(), sid)
	require.Equal(t, http.StatusOK, who.Code, who.Body.String())
	assert.Contains(t, who.Body.String(), `"source":"oidc"`)

	root, err := a.svcs.Users.ByEmail(ctx, "root@example.com")
	require.NoError(t, err)
	assert.Equal(t, a.idp.Issuer(), root.IDPIssuer, "the link keeps the normalized issuer")
	assert.Equal(t, "sub-root", root.IDPSubject)
	assert.True(t, root.HasPassword(), "linking keeps the password")

	again := a.signIn(t, oidcrptest.User{Subject: "sub-root", Email: "root.new@example.com", EmailVerified: true})
	require.Equal(t, http.StatusSeeOther, again.Code, "the sub alone signs in: %s", again.Body.String())

	other := a.signIn(t, oidcrptest.User{Subject: "sub-other", Email: "root@example.com", EmailVerified: true})
	assert.Equal(t, http.StatusBadRequest, other.Code)
	assert.Contains(t, other.Body.String(), "AUT007")

	unverified := a.signIn(t, oidcrptest.User{Subject: "sub-new", Email: "new@example.com"})
	assert.Equal(t, http.StatusForbidden, unverified.Code)
	assert.Contains(t, unverified.Body.String(), "Verify your email")

	stranger := a.signIn(t, oidcrptest.User{Subject: "sub-new", Email: "new@example.com", EmailVerified: true})
	assert.Equal(t, http.StatusForbidden, stranger.Code, "selfhosted needs an invite")
	assert.Contains(t, stranger.Body.String(), "You need an invite")

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	out := serve(a.h, req, sid)
	assert.Equal(t, http.StatusSeeOther, out.Code)
	assert.Equal(t, "/login", out.Header().Get("Location"), "no end_session_endpoint at this provider")
}

// With passwords off the provider is the only way in.
func TestOIDCSignIn_PasswordsOff(t *testing.T) {
	a := newOIDCApp(t, false)
	login := serve(a.h, httptest.NewRequest(http.MethodGet, "/login", nil))
	assert.Contains(t, login.Body.String(), "Continue with SSO")
	assert.NotContains(t, login.Body.String(), `name="password"`)
	assert.Equal(t, http.StatusMethodNotAllowed, serve(a.h, formPost("/login", nil)).Code)

	rec := a.signIn(t, oidcrptest.User{Subject: "sub-root", Email: "root@example.com", EmailVerified: true})
	assert.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
}

func whoamiRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth.v1.AuthService/Whoami", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	return req
}
