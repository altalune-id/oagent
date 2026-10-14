//go:build integration

package boot

import (
	"context"
	"encoding/base64"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/health"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	mcpinternal "altalune.id/oagent/internal/mcp"
	"altalune.id/oagent/internal/permission"
)

type lockdown struct {
	h    http.Handler
	svcs *Services
	cfg  config.Config
}

func newLockdown(t *testing.T, cfg config.Config) *lockdown {
	t.Helper()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	svcs, cfg := migratedServices(t, cfg)
	svcs.Caps.EnvGenesis = cfg.Auth.Mode == config.AuthModeLocal
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	svcs.InstallRequired = required

	console := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("console")) })
	h := buildHTTPHandler(httpSurfaces{
		probes:          health.NewWeb(health.NewService()),
		api:             buildAPIHandler(svcs, nil, cfg.API, "", testLogger()),
		mcp:             buildMCPHandler(buildMCPServer(config.MCPConfig{Enabled: true}, svcs, testLogger()), svcs, testLogger()),
		console:         console,
		consoleGate:     buildConsoleGate(cfg, svcs),
		routes:          buildRoutes(cfg, svcs, nil, nil),
		session:         buildSessionMiddleware(svcs, nil),
		installRequired: required,
	})
	return &lockdown{h: h, svcs: svcs, cfg: cfg}
}

func (l *lockdown) do(method, path string, header http.Header, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	l.h.ServeHTTP(rec, req)
	return rec
}

func connectJSON() http.Header {
	return http.Header{"Content-Type": {"application/json"}}
}

func mcpJSON() http.Header {
	return http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json, text/event-stream"}}
}

const listTools = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func (l *lockdown) sessionCookie(t *testing.T) string {
	t.Helper()
	p, err := l.svcs.Auth.LoginLocal(t.Context(), authCredentials("root@example.com", "genesis-password"))
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, l.svcs.Sessions.Save(t.Context(), sid, p, time.Now().Add(time.Hour)))
	return session.CookieName + "=" + session.Sign(l.svcs.StateSecret, sid)
}

func localLockdown(t *testing.T) *lockdown {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	return newLockdown(t, cfg)
}

// Review Focus 2: no credential is refused on every surface; sign-in pages, legal, static and probes stay open.
func TestLockdown_NoCredential(t *testing.T) {
	l := localLockdown(t)

	rec := l.do(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", connectJSON(), `{}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "/api")

	rec = l.do(http.MethodPost, "/mcp", mcpJSON(), listTools)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "/mcp")
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))

	for path, want := range map[string]string{
		"/console/":             "/login?return_to=%2Fconsole%2F",
		"/console/o/x/agents":   "/login?return_to=%2Fconsole%2Fo%2Fx%2Fagents",
		"/console/o/x?tab=keys": "/login?return_to=%2Fconsole%2Fo%2Fx%3Ftab%3Dkeys",
	} {
		rec = l.do(http.MethodGet, path, nil, "")
		assert.Equal(t, http.StatusSeeOther, rec.Code, path)
		assert.Equal(t, want, rec.Header().Get("Location"), path)
	}

	for _, path := range []string{"/login", "/terms", "/privacy", "/static/favicon.ico", "/healthz", "/readyz"} {
		rec = l.do(http.MethodGet, path, nil, "")
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
	rec = l.do(http.MethodGet, "/install", nil, "")
	assert.Equal(t, http.StatusSeeOther, rec.Code, "/install")
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestLockdown_CredentialsWork(t *testing.T) {
	l := localLockdown(t)
	cookie := l.sessionCookie(t)

	rec := l.do(http.MethodGet, "/console/o/x/agents", http.Header{"Cookie": {cookie}}, "")
	assert.Equal(t, http.StatusOK, rec.Code, "console with a session")
	h := connectJSON()
	h.Set("Cookie", cookie)
	rec = l.do(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", h, `{}`)
	assert.Equal(t, http.StatusOK, rec.Code, "api with a session: %s", rec.Body.String())

	sys, err := l.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	owner, err := l.svcs.Users.ByEmail(t.Context(), "root@example.com")
	require.NoError(t, err)
	exp := time.Now().Add(time.Hour)
	_, personal, err := l.svcs.APIKeys.Create(t.Context(), apikey.CreateRequest{
		OrgID: sys.ID, Kind: apikey.KindPersonal, Name: "laptop", Scopes: []string{"agent:read"}, AllProjects: true, ExpiresAt: &exp, CreatedBy: owner.ID,
	})
	require.NoError(t, err)
	h = mcpJSON()
	h.Set("Authorization", "Bearer "+personal)
	rec = l.do(http.MethodPost, "/mcp", h, listTools)
	assert.Equal(t, http.StatusOK, rec.Code, "/mcp with a personal key")
	assert.Contains(t, rec.Body.String(), "auth_whoami")

	h = mcpJSON()
	h.Set("Cookie", cookie)
	rec = l.do(http.MethodPost, "/mcp", h, listTools)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "/mcp never accepts the session cookie")
}

// Review Focus 3: a cookie whose session is gone gets the login form, never a redirect loop.
func TestLockdown_StaleCookieNoLoop(t *testing.T) {
	l := localLockdown(t)
	stale := session.CookieName + "=" + session.Sign(l.svcs.StateSecret, "deleted-session")

	rec := l.do(http.MethodGet, "/console/o/x/agents", http.Header{"Cookie": {stale}}, "")
	require.Equal(t, http.StatusSeeOther, rec.Code)
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/login", loc.Path)

	rec = l.do(http.MethodGet, loc.String(), http.Header{"Cookie": {stale}}, "")
	assert.Equal(t, http.StatusOK, rec.Code, "the login form renders")
	assert.Contains(t, rec.Body.String(), "<form")

	h := connectJSON()
	h.Set("Cookie", stale)
	rec = l.do(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", h, `{}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the console's API call answers 401, which the SPA turns into one redirect")
}

func TestLockdown_StdioStillTrusted(t *testing.T) {
	ctx := session.PrincipalInto(context.Background(), session.System())
	notPerson := slices.DeleteFunc(allMCPScopes(), func(s string) bool { return s == authz.PersonOnly })
	assert.Equal(t, notPerson, mcpinternal.ScopesFromContext(ctx), "stdio holds everything but the person-only marker")
	assert.True(t, authz.ReachWholeOrg(ctx, uuid.New()), "stdio reaches every org")
	assert.Empty(t, mcpinternal.ScopesFromContext(context.Background()), "no Principal holds no scope")
}

func allMCPScopes() []string {
	seen := map[string]bool{}
	var out []string
	for _, perms := range permission.Table() {
		for _, s := range perms {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	slices.Sort(out)
	return out
}

func basicLockdown(t *testing.T) *lockdown {
	t.Helper()
	cfg := devLocalConfig()
	cfg.Auth = config.AuthConfig{Mode: config.AuthModeBasic, Basic: config.BasicConfig{User: "ops", Password: "basic-password"}}
	return newLockdown(t, cfg)
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestLockdown_BasicMode(t *testing.T) {
	l := basicLockdown(t)

	rec := l.do(http.MethodGet, "/console/", nil, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, `Basic realm="oagent"`, rec.Header().Get("WWW-Authenticate"))

	rec = l.do(http.MethodGet, "/console/", http.Header{"Authorization": {basicAuth("ops", "basic-password")}}, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = l.do(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", connectJSON(), `{}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, `Basic realm="oagent"`, rec.Header().Get("WWW-Authenticate"))

	h := connectJSON()
	h.Set("Authorization", basicAuth("ops", "basic-password"))
	rec = l.do(http.MethodPost, "/api/org.v1.OrgService/ListOrgs", h, `{}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
