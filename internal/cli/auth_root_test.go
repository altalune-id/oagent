package cli

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/web/layout"
)

const (
	testEmail    = "ada@example.com"
	testPassword = "correct horse battery"
	testKey      = "oagent_good"
)

type fakeSignIn struct {
	store session.Store
	p     session.Principal
}

func (f fakeSignIn) LoginLocal(_ context.Context, c auth.Credentials) (session.Principal, error) {
	if c.Email != testEmail || c.Password != testPassword {
		return session.Principal{}, &auth.InvalidCredentialsError{}
	}
	return f.p, nil
}

func (f fakeSignIn) Logout(ctx context.Context, sid string) error { return f.store.Delete(ctx, sid) }

type storeAuthn struct{ store session.Store }

func (a storeAuthn) Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error) {
	switch c.Kind {
	case authn.KindAPIKey:
		if c.Token == testKey {
			return session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), Scopes: []string{"org:read"}}, nil
		}
	case authn.KindSession:
		if p, ok, err := a.store.Load(ctx, c.Token); err == nil && ok {
			p.Scopes = []string{"*"}
			return p, nil
		}
	}
	return session.Principal{}, &authn.UnauthorizedError{}
}

type testServer struct {
	URL    string
	secret []byte
	store  session.Store
	orgID  string

	mu       sync.Mutex
	requests int
	cookie   string
	authz    string
}

func (s *testServer) seen() (int, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.cookie, s.authz
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{secret: bytes.Repeat([]byte{7}, 32), store: session.NewMemoryStore()}
	ids := fakes.NewPublicIDs()
	acme := &org.Org{ID: uuid.New(), Slug: "acme", Name: "Acme"}
	acme.PublicID = ids.Org(acme.ID)
	ts.orgID = acme.PublicID
	p := session.Principal{Source: session.SourceLocal, UserID: uuid.New(), Email: testEmail, Name: "Ada", ActiveOrgID: acme.ID}
	ids.User(p.UserID)

	api := controlplane.New(controlplane.Services{
		Orgs:     org.NewService(&fakes.OrgStore{Org: acme}, fakes.Transactor{}, nil),
		Projects: project.NewService(&fakes.ProjectStore{}, nil),
		Users:    user.NewService(fakes.NewUserStore(), nil),
		Refs:     ids,
	}, nil, controlplane.Options{Authn: authn.InterceptorOptions{
		Chain:        authn.Chain{storeAuthn{store: ts.store}},
		KeyPrefix:    "oagent_",
		CookieSecret: ts.secret,
		Permissions:  permission.Table(),
	}}).Handler("")
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	auth.NewWeb(layout.Kit{}, fakeSignIn{store: ts.store, p: p}, auth.CookieSessions{
		Store: ts.store, Secret: ts.secret, Cookie: session.CookieOptions{Path: "/"}, TTL: time.Hour,
	}, ids).Register(mux)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.requests++
		ts.cookie, ts.authz = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		ts.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	ts.URL = srv.URL
	return ts
}

func cliEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	clearCLIEnv(t)
	path := filepath.Join(dir, "home", ".oagent", "session.json")
	t.Setenv("OAGENT_CLI_SESSION_FILE", path)
	return path
}

func remoteBoot(_ context.Context, _ config.Config, _ *slog.Logger, baseURL string, cred apiclient.Credential) (*apiclient.Client, error) {
	return apiclient.New(baseURL, cred), nil
}

func runCLI(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd(nil, remoteBoot)
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), errOut.String(), err
}

func login(t *testing.T, srv *testServer, profileName string) profile {
	t.Helper()
	_, _, err := runCLI(t, testPassword+"\n", "auth", "login", "--url", srv.URL, "--profile", profileName, "--email", testEmail, "--password-stdin")
	require.NoError(t, err)
	f, err := loadSessionFile(os.Getenv("OAGENT_CLI_SESSION_FILE"))
	require.NoError(t, err)
	return f.Profiles[profileName]
}

func TestAuthLogin_SavesAPrivateProfile(t *testing.T) {
	path := cliEnv(t)
	srv := newTestServer(t)

	out, errOut, err := runCLI(t, testEmail+"\n"+testPassword+"\n", "auth", "login", "--url", srv.URL, "--profile", "dev")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dir, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())

	f, err := loadSessionFile(path)
	require.NoError(t, err)
	p := f.Profiles["dev"]
	assert.Equal(t, srv.URL, p.URL)
	sid, ok := session.ReadCookie(http.Header{"Cookie": {session.CookieName + "=" + p.Session}}, srv.secret)
	require.True(t, ok, "the stored session is the server's signed cookie value")
	_, live, err := srv.store.Load(t.Context(), sid)
	require.NoError(t, err)
	assert.True(t, live)
	assert.True(t, p.ExpiresAt.After(time.Now()))
	assert.Equal(t, testEmail, p.Principal.Email)
	assert.Equal(t, "acme", p.Principal.ActiveOrgSlug, "filled by whoami after login")

	assert.Contains(t, out, "Signed in to "+srv.URL+" as "+testEmail+" (profile dev)")
	for _, secret := range []string{p.Session, sid, testPassword} {
		assert.NotContains(t, out, secret)
		assert.NotContains(t, errOut, secret)
	}
}

func TestAuthLogin_PasswordStdinWithoutTTY(t *testing.T) {
	cliEnv(t)
	p := login(t, newTestServer(t), "default")
	assert.NotEmpty(t, p.Session)
}

func TestAuthLogin_WrongPassword(t *testing.T) {
	path := cliEnv(t)
	srv := newTestServer(t)

	_, _, err := runCLI(t, "wrong\n", "auth", "login", "--url", srv.URL, "--email", testEmail, "--password-stdin")
	require.Error(t, err)
	assert.Equal(t, ExitUnauth, ExitCodeFor(err))
	assert.Contains(t, err.Error(), "invalid email or password")
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "no session file on a failed login")
}

func TestAuthProfiles_StayApart(t *testing.T) {
	cliEnv(t)
	a, b := newTestServer(t), newTestServer(t)
	pa := login(t, a, "a")
	pb := login(t, b, "b")

	_, _, err := runCLI(t, "", "auth", "whoami", "--profile", "a")
	require.NoError(t, err)
	_, cookie, _ := a.seen()
	assert.Equal(t, session.CookieName+"="+pa.Session, cookie)

	_, _, err = runCLI(t, "", "auth", "whoami", "--profile", "b")
	require.NoError(t, err)
	_, cookie, _ = b.seen()
	assert.Equal(t, session.CookieName+"="+pb.Session, cookie)
}

// SECURITY: Review Focus 1 — a profile's session never reaches another server.
func TestProfileSession_NotSentToAnotherServer(t *testing.T) {
	for name, redirect := range map[string]func(t *testing.T, url string) []string{
		"--url": func(_ *testing.T, url string) []string { return []string{"--url", url} },
		"OAGENT_URL": func(t *testing.T, url string) []string {
			t.Setenv("OAGENT_URL", url)
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			cliEnv(t)
			a, b := newTestServer(t), newTestServer(t)
			login(t, a, "default")

			_, _, err := runCLI(t, "", append([]string{"org", "list"}, redirect(t, b.URL)...)...)
			require.True(t, IsProfileHostMismatchError(err), "%v", err)
			assert.Equal(t, ExitUnauth, ExitCodeFor(err))
			requests, _, _ := b.seen()
			assert.Zero(t, requests, "nothing is sent to the other server")
		})
	}
}

func TestProfileSession_ExpiredIsNotSent(t *testing.T) {
	path := cliEnv(t)
	srv := newTestServer(t)
	login(t, srv, "default")
	f, err := loadSessionFile(path)
	require.NoError(t, err)
	p := f.Profiles["default"]
	p.ExpiresAt = time.Now().Add(-time.Minute)
	f.Profiles["default"] = p
	require.NoError(t, saveSessionFile(path, f))
	before, _, _ := srv.seen()

	_, _, err = runCLI(t, "", "auth", "whoami")
	require.True(t, IsSessionExpiredError(err), "%v", err)
	after, _, _ := srv.seen()
	assert.Equal(t, before, after, "no request is sent")
}

func TestToken_BeatsTheProfile(t *testing.T) {
	cliEnv(t)
	srv := newTestServer(t)
	login(t, srv, "default")

	_, _, _ = runCLI(t, "", "org", "list", "--token", testKey)
	_, cookie, authz := srv.seen()
	assert.Equal(t, "Bearer "+testKey, authz)
	assert.Empty(t, cookie)
}

func TestAuthWhoami_RefreshesTheProfile(t *testing.T) {
	path := cliEnv(t)
	srv := newTestServer(t)
	login(t, srv, "default")
	f, err := loadSessionFile(path)
	require.NoError(t, err)
	p := f.Profiles["default"]
	p.Principal.Name, p.Principal.ActiveOrgSlug = "stale", ""
	f.Profiles["default"] = p
	require.NoError(t, saveSessionFile(path, f))

	out, _, err := runCLI(t, "", "auth", "whoami")
	require.NoError(t, err)
	assert.Contains(t, out, "Ada")
	assert.Contains(t, out, "acme")
	assert.NotContains(t, out, p.Session)

	f, err = loadSessionFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Ada", f.Profiles["default"].Principal.Name)
	assert.Equal(t, "acme", f.Profiles["default"].Principal.ActiveOrgSlug)
}

func TestAuthWhoami_JSON(t *testing.T) {
	cliEnv(t)
	srv := newTestServer(t)
	login(t, srv, "default")
	out, _, err := runCLI(t, "", "auth", "whoami", "--output", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"email": "`+testEmail+`"`)
	assert.Contains(t, out, `"active_org_id": "`+srv.orgID+`"`)
	assert.Contains(t, out, `"active_org_slug": "acme"`)
}

func TestAuthLogout_EndsTheServerSessionAndOneProfile(t *testing.T) {
	path := cliEnv(t)
	srv := newTestServer(t)
	old := login(t, srv, "a")
	login(t, srv, "b")

	out, _, err := runCLI(t, "", "auth", "logout", "--profile", "a")
	require.NoError(t, err)
	assert.Contains(t, out, "Signed out of "+srv.URL+" (profile a)")

	f, err := loadSessionFile(path)
	require.NoError(t, err)
	assert.NotContains(t, f.Profiles, "a")
	assert.Contains(t, f.Profiles, "b")

	_, err = apiclient.New(srv.URL, apiclient.Credential{Session: old.Session}).Auth.
		Whoami(t.Context(), connect.NewRequest(&authv1.WhoamiRequest{}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestAuthLogout_NotSignedIn(t *testing.T) {
	cliEnv(t)
	out, _, err := runCLI(t, "", "auth", "logout", "--profile", "x")
	require.NoError(t, err)
	assert.Contains(t, out, "not signed in (profile x)")
}

func TestCorruptSessionFile_NamesThePath(t *testing.T) {
	path := cliEnv(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, _, err := runCLI(t, "", "org", "list", "--url", "http://127.0.0.1:1")
	require.True(t, IsSessionFileError(err), "%v", err)
	assert.Contains(t, err.Error(), path)
}
