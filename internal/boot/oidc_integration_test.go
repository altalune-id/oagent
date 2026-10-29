//go:build integration

package boot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altalune-id/oidcrp/oidcrptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
)

type oidcApp struct {
	idp  *oidcrptest.IDP
	svcs *Services
	h    http.Handler
}

func newOIDCApp(t *testing.T, passwords bool) *oidcApp {
	t.Helper()
	idp := oidcrptest.New(t)
	cfg := oidcConfig(idp)
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	return startOIDCApp(t, idp, cfg, passwords)
}

func oidcConfig(idp *oidcrptest.IDP) config.Config {
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.OIDC = config.OIDCConfig{Issuer: idp.Issuer(), ClientID: "oagent", Scopes: []string{"openid", "profile", "email"}, ButtonLabel: "Continue with SSO"}
	return cfg
}

func startOIDCApp(t *testing.T, idp *oidcrptest.IDP, cfg config.Config, passwords bool) *oidcApp {
	t.Helper()
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
	return a.signInFrom(t, user, "")
}

func (a *oidcApp) signInFrom(t *testing.T, user oidcrptest.User, invite string) *httptest.ResponseRecorder {
	t.Helper()
	a.idp.User = user
	start := serve(a.h, httptest.NewRequest(http.MethodGet, "/login/oidc?return_to=/console/&invite="+invite, nil))
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

// SECURITY: an invite joins only its own verified email, once; another email joins nothing.
func TestOIDCSignIn_FromAnInvite(t *testing.T) {
	a := newOIDCApp(t, false)
	ctx := t.Context()
	sys, err := a.svcs.Orgs.System(ctx)
	require.NoError(t, err)
	c, err := a.svcs.Invites.Create(ctx, member.Requester{Operator: true}, sys.ID, "Bob@Example.com", member.RoleMember)
	require.NoError(t, err)

	eve := a.signInFrom(t, oidcrptest.User{Subject: "sub-eve", Email: "eve@example.com", EmailVerified: true}, c.Invite.PublicID)
	assert.Equal(t, http.StatusForbidden, eve.Code, "someone else's invite: selfhosted needs one of their own")
	_, err = a.svcs.Users.ByEmail(ctx, "eve@example.com")
	assert.Error(t, err, "no account for eve")

	bob := a.signInFrom(t, oidcrptest.User{Subject: "sub-bob", Email: "bob@example.com", EmailVerified: true, Name: "Bob"}, c.Invite.PublicID)
	require.Equal(t, http.StatusSeeOther, bob.Code, bob.Body.String())
	assert.Equal(t, "/console/", bob.Header().Get("Location"))
	u, err := a.svcs.Users.ByEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, "sub-bob", u.IDPSubject)
	assert.False(t, u.HasPassword(), "an OIDC account has no password")
	m, err := a.svcs.Members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleMember, m.Role)
	stored, err := a.svcs.Invites.Landing(ctx, c.Invite.PublicID)
	require.NoError(t, err)
	assert.NotNil(t, stored.AcceptedAt, "the invite is used up")

	again := a.signInFrom(t, oidcrptest.User{Subject: "sub-bob", Email: "bob@example.com", EmailVerified: true}, c.Invite.PublicID)
	assert.Equal(t, http.StatusSeeOther, again.Code, "a used invite is skipped; the linked account signs in")
}

// SECURITY: a verified email alone claims only the email-only owner seat install leaves, once; any other invite needs its link.
func TestOIDCSignIn_OnlyTheEmailOnlyOwnerSeat(t *testing.T) {
	idp := oidcrptest.New(t)
	cfg := oidcConfig(idp)
	cfg.Auth.Password.Enabled = new(false)
	cfg.Auth.Genesis = config.GenesisConfig{Email: "Owner@Example.com"}
	a := startOIDCApp(t, idp, cfg, false)
	ctx := t.Context()
	required, err := a.svcs.Install.Required(ctx, false)
	require.NoError(t, err)
	assert.False(t, required, "the pending owner seat closes /install")
	sys, err := a.svcs.Orgs.System(ctx)
	require.NoError(t, err)
	seats := oidcInvites{invites: a.svcs.Invites}
	refused := func(email string) {
		t.Helper()
		seat, err := seats.OwnerSeat(ctx, sys.ID, email)
		require.NoError(t, err)
		assert.Nil(t, seat, email)
		rec := a.signIn(t, oidcrptest.User{Subject: "sub-" + email, Email: email, EmailVerified: true})
		assert.Equal(t, http.StatusForbidden, rec.Code, email)
		assert.Contains(t, rec.Body.String(), "You need an invite", email)
	}

	_, err = a.svcs.Invites.Create(ctx, member.Requester{Operator: true}, sys.ID, "mem@example.com", member.RoleMember)
	require.NoError(t, err)
	refused("mem@example.com")
	_, err = a.svcs.Invites.Create(ctx, member.Requester{Operator: true}, sys.ID, "op@example.com", member.RoleOwner)
	require.NoError(t, err)
	refused("op@example.com")
	refused("eve@example.com")

	rec := a.signIn(t, oidcrptest.User{Subject: "sub-owner", Email: "owner@example.com", EmailVerified: true, Name: "Owner"})
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	assert.Equal(t, "/console/", rec.Header().Get("Location"))
	owner, err := a.svcs.Users.ByEmail(ctx, "owner@example.com")
	require.NoError(t, err)
	assert.False(t, owner.HasPassword())
	m, err := a.svcs.Members.Find(ctx, sys.ID, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	seat, err := seats.OwnerSeat(ctx, sys.ID, "owner@example.com")
	require.NoError(t, err)
	assert.Nil(t, seat, "the seat is used up")

	_, err = a.svcs.Invites.Create(ctx, member.Requester{UserID: owner.ID}, sys.ID, "co@example.com", member.RoleOwner)
	require.NoError(t, err)
	refused("co@example.com")

	twin := a.signIn(t, oidcrptest.User{Subject: "sub-twin", Email: "owner@example.com", EmailVerified: true})
	assert.Equal(t, http.StatusBadRequest, twin.Code, "the claimed account stays linked to the first sub")
	require.NoError(t, firstBoot(ctx, cfg, a.svcs, testLogger(), false), "a second boot changes nothing")
	owners, err := a.svcs.Members.CountOwners(ctx, sys.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
}
