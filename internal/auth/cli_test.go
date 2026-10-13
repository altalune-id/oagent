package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/render"
)

type memProfiles struct {
	s     *auth.CLISession
	url   string
	saved int
}

func (m *memProfiles) Target(*cobra.Command) (string, string, error) {
	if m.url != "" {
		return "dev", m.url, nil
	}
	return "dev", "http://oagent.test", nil
}

func (m *memProfiles) Current(*cobra.Command) (string, *auth.CLISession, error) {
	if m.s == nil {
		return "dev", nil, nil
	}
	s := *m.s
	return "dev", &s, nil
}

func (m *memProfiles) Save(_ *cobra.Command, s auth.CLISession) error {
	m.s = &s
	m.saved++
	return nil
}

func (m *memProfiles) Remove(*cobra.Command) error {
	m.s = nil
	return nil
}

type whoamiOnly struct {
	authv1connect.AuthServiceClient
	resp *authv1.WhoamiResponse
	err  error
}

func (c whoamiOnly) Whoami(context.Context, *connect.Request[authv1.WhoamiRequest]) (*connect.Response[authv1.WhoamiResponse], error) {
	if c.err != nil {
		return nil, c.err
	}
	return connect.NewResponse(c.resp), nil
}

type authHarness struct {
	profiles    *memProfiles
	client      whoamiOnly
	fromProfile bool
	clientErr   error
	sessionErr  error
	sessionURL  string
	sessionSID  string
}

func newAuthHarness(profiles *memProfiles) *authHarness {
	return &authHarness{
		profiles: profiles,
		client:   whoamiOnly{resp: &authv1.WhoamiResponse{Source: "local", Email: "ada@example.com", ActiveOrgSlug: "acme"}},
	}
}

func (h *authHarness) run(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := auth.NewCmd(
		h.profiles,
		func(*cobra.Command) (authv1connect.AuthServiceClient, bool, error) {
			return h.client, h.fromProfile, h.clientErr
		},
		func(_ *cobra.Command, url, sid string) (authv1connect.AuthServiceClient, error) {
			h.sessionURL, h.sessionSID = url, sid
			return h.client, h.sessionErr
		},
	)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(t.Context())
	return out.String(), errOut.String(), err
}

func runAuth(t *testing.T, profiles *memProfiles, fromProfile bool, args ...string) string {
	t.Helper()
	h := newAuthHarness(profiles)
	h.fromProfile = fromProfile
	out, errOut, err := h.run(t, "", args...)
	require.NoError(t, err)
	return out + errOut
}

func TestWhoami_RefreshesOnlyTheProfileItUsed(t *testing.T) {
	profiles := &memProfiles{s: &auth.CLISession{URL: "http://oagent.test", Session: "sid"}}

	runAuth(t, profiles, false, "whoami")
	assert.Zero(t, profiles.saved, "a token credential leaves the profile alone")

	out := runAuth(t, profiles, true, "whoami")
	assert.Equal(t, 1, profiles.saved)
	assert.Equal(t, "acme", profiles.s.Principal.ActiveOrgSlug)
	assert.Equal(t, "sid", profiles.s.Session)
	assert.Contains(t, out, "ada@example.com")
	assert.NotContains(t, out, "sid")
}

func TestWhoami_FromProfileWithoutSessionSavesNothing(t *testing.T) {
	profiles := &memProfiles{}
	runAuth(t, profiles, true, "whoami")
	assert.Zero(t, profiles.saved)
	assert.Nil(t, profiles.s)
}

func TestLogout_NotSignedIn(t *testing.T) {
	out := runAuth(t, &memProfiles{}, false, "logout")
	assert.Contains(t, out, "not signed in (profile dev)")
}

type loginServer struct {
	*httptest.Server
	form        map[string]string
	contentType string
	accept      string
	cookie      string
	path        string
	followed    bool
}

func newLoginServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *loginServer {
	t.Helper()
	s := &loginServer{form: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/elsewhere", func(w http.ResponseWriter, _ *http.Request) {
		s.followed = true
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.path = r.Method + " " + r.URL.Path
		s.contentType = r.Header.Get("Content-Type")
		s.accept = r.Header.Get("Accept")
		if c, err := r.Cookie(session.CookieName); err == nil {
			s.cookie = c.Value
		}
		if err := r.ParseForm(); err == nil {
			for k := range r.PostForm {
				s.form[k] = r.PostForm.Get(k)
			}
		}
		handle(w, r)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

var loginExpires = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func loginOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"session":    "signed-sid",
		"expires_at": loginExpires,
		"principal": map[string]any{
			"source": "local", "user_id": "u-1", "email": "ada@example.com", "name": "Ada",
			"active_org_id": "o-1", "permissions": []string{"agent:read"}, "operator": true,
		},
	})
}

func TestLogin_SavesSessionAndWhoamiPrincipal(t *testing.T) {
	srv := newLoginServer(t, loginOK)
	profiles := &memProfiles{url: srv.URL + "/"}
	h := newAuthHarness(profiles)

	out, errOut, err := h.run(t, "correct horse battery\n", "login", "--email", "  ada@example.com ", "--password-stdin")
	require.NoError(t, err)

	assert.Equal(t, "POST /login", srv.path)
	assert.Equal(t, "application/x-www-form-urlencoded", srv.contentType)
	assert.Equal(t, "application/json", srv.accept)
	assert.Equal(t, map[string]string{"email": "ada@example.com", "password": "correct horse battery"}, srv.form)

	assert.Equal(t, srv.URL+"/", h.sessionURL)
	assert.Equal(t, "signed-sid", h.sessionSID)

	require.NotNil(t, profiles.s)
	assert.Equal(t, srv.URL+"/", profiles.s.URL)
	assert.Equal(t, "signed-sid", profiles.s.Session)
	assert.True(t, loginExpires.Equal(profiles.s.ExpiresAt))
	assert.Equal(t, auth.CLIPrincipal{Source: "local", Email: "ada@example.com", ActiveOrgSlug: "acme"}, profiles.s.Principal)

	assert.Equal(t, "Signed in to "+srv.URL+"/ as ada@example.com (profile dev)\n", out)
	assert.Empty(t, errOut)
	assert.NotContains(t, out+errOut, "correct horse battery")
	assert.NotContains(t, out+errOut, "signed-sid")
}

func TestLogin_PromptsForEmailAndPassword(t *testing.T) {
	srv := newLoginServer(t, loginOK)
	profiles := &memProfiles{url: srv.URL}
	h := newAuthHarness(profiles)

	_, errOut, err := h.run(t, " ada@example.com \r\ncorrect horse battery\n", "login")
	require.NoError(t, err)
	assert.Equal(t, "Email: ", errOut)
	assert.Equal(t, map[string]string{"email": "ada@example.com", "password": "correct horse battery"}, srv.form)
	assert.Equal(t, 1, profiles.saved)
}

func TestLogin_WhoamiFailureKeepsLoginPrincipal(t *testing.T) {
	tests := []struct {
		name  string
		setup func(h *authHarness)
	}{
		{name: "session client", setup: func(h *authHarness) { h.sessionErr = errors.New("no client") }},
		{name: "whoami call", setup: func(h *authHarness) { h.client.err = errors.New("no client") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newLoginServer(t, loginOK)
			profiles := &memProfiles{url: srv.URL}
			h := newAuthHarness(profiles)
			tt.setup(h)

			out, errOut, err := h.run(t, "pw\n", "login", "--email", "ada@example.com")
			require.NoError(t, err)
			assert.Equal(t, "warn: signed in, but whoami failed: no client\n", errOut)
			require.NotNil(t, profiles.s)
			assert.Equal(t, auth.CLIPrincipal{
				Source: "local", UserID: "u-1", Email: "ada@example.com", Name: "Ada",
				ActiveOrgID: "o-1", Permissions: []string{"agent:read"}, Operator: true,
			}, profiles.s.Principal)
			assert.Contains(t, out, "as ada@example.com")
		})
	}
}

func TestLogin_Failures(t *testing.T) {
	tests := []struct {
		name    string
		handle  func(w http.ResponseWriter, r *http.Request)
		wantErr string
		check   func(t *testing.T, err error, srv *loginServer)
	}{
		{
			name: "invalid credentials",
			handle: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "server detail", http.StatusUnauthorized)
			},
			wantErr: "invalid email or password",
			check: func(t *testing.T, err error, _ *loginServer) {
				ae, ok := apperror.AsAppError(err)
				require.True(t, ok)
				assert.Equal(t, apperror.CodeAuthInvalidCredentials, ae.Code())
			},
		},
		{
			name: "other status hides the body",
			handle: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "server detail", http.StatusInternalServerError)
			},
			wantErr: "answered 500 Internal Server Error",
		},
		{
			name: "redirect is not followed",
			handle: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
			},
			wantErr: "answered 302 Found",
			check: func(t *testing.T, _ error, srv *loginServer) {
				assert.False(t, srv.followed)
			},
		},
		{
			name: "undecodable body",
			handle: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("not json"))
			},
			wantErr: "login: decode response:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newLoginServer(t, tt.handle)
			profiles := &memProfiles{url: srv.URL}
			h := newAuthHarness(profiles)

			_, _, err := h.run(t, "pw\n", "login", "--email", "ada@example.com")
			require.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "server detail")
			assert.Zero(t, profiles.saved)
			assert.Empty(t, h.sessionSID)
			if tt.check != nil {
				tt.check(t, err, srv)
			}
		})
	}
}

func TestLogin_ServerUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	profiles := &memProfiles{url: srv.URL}

	_, _, err := newAuthHarness(profiles).run(t, "pw\n", "login", "--email", "ada@example.com")
	require.ErrorContains(t, err, "login:")
	assert.Zero(t, profiles.saved)
}

func TestLogin_BadURL(t *testing.T) {
	profiles := &memProfiles{url: "http://bad host"}
	_, _, err := newAuthHarness(profiles).run(t, "pw\n", "login", "--email", "ada@example.com")
	require.ErrorContains(t, err, "login:")
	assert.Zero(t, profiles.saved)
}

func TestLogin_ReadErrors(t *testing.T) {
	tests := []struct {
		name, stdin, wantErr string
		args                 []string
	}{
		{name: "no email", args: []string{"login"}, wantErr: "login: read email:"},
		{name: "no password", args: []string{"login", "--email", "ada@example.com"}, wantErr: "login: read password:"},
		{name: "no password after email", stdin: "ada@example.com\n", args: []string{"login"}, wantErr: "login: read password:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profiles := &memProfiles{}
			_, _, err := newAuthHarness(profiles).run(t, tt.stdin, tt.args...)
			require.ErrorContains(t, err, tt.wantErr)
			assert.Zero(t, profiles.saved)
		})
	}
}

func TestLogout_EndsServerSessionAndRemovesProfile(t *testing.T) {
	srv := newLoginServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusSeeOther) })
	profiles := &memProfiles{s: &auth.CLISession{URL: srv.URL + "/", Session: "signed-sid"}}

	out, errOut, err := newAuthHarness(profiles).run(t, "", "logout")
	require.NoError(t, err)
	assert.Equal(t, "POST /logout", srv.path)
	assert.Equal(t, "signed-sid", srv.cookie)
	assert.Nil(t, profiles.s)
	assert.Equal(t, "Signed out of "+srv.URL+"/ (profile dev)\n", out)
	assert.Empty(t, errOut)
}

func TestLogout_ServerErrorStillRemovesProfile(t *testing.T) {
	tests := []struct {
		name string
		url  func(t *testing.T) string
	}{
		{name: "unreachable", url: func(*testing.T) string {
			srv := httptest.NewServer(http.NotFoundHandler())
			srv.Close()
			return srv.URL
		}},
		{name: "bad url", url: func(*testing.T) string { return "http://bad host" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.url(t)
			profiles := &memProfiles{s: &auth.CLISession{URL: u, Session: "signed-sid"}}

			out, errOut, err := newAuthHarness(profiles).run(t, "", "logout")
			require.NoError(t, err)
			assert.Contains(t, errOut, "warn: the server session was not ended:")
			assert.NotContains(t, errOut, "signed-sid")
			assert.Nil(t, profiles.s)
			assert.Equal(t, "Signed out of "+u+" (profile dev)\n", out)
		})
	}
}

func TestWhoami_Errors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(h *authHarness)
	}{
		{name: "client", setup: func(h *authHarness) { h.clientErr = errors.New("boom") }},
		{name: "call", setup: func(h *authHarness) { h.client.err = errors.New("boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profiles := &memProfiles{s: &auth.CLISession{Session: "sid"}}
			h := newAuthHarness(profiles)
			h.fromProfile = true
			tt.setup(h)

			out, _, err := h.run(t, "", "whoami")
			require.EqualError(t, err, "boom")
			assert.NotContains(t, out, "ada@example.com")
			assert.Zero(t, profiles.saved)
		})
	}
}

var fullWhoami = &authv1.WhoamiResponse{
	Source: "apikey", UserId: "u-1", Email: "ada@example.com", Name: "Ada", KeyHint: "a1b2", OwnerId: "u-1",
	ActiveOrgId: "o-1", ActiveOrgSlug: "acme", Permissions: []string{"agent:read", "agent:write"}, Operator: true,
}

func TestWhoami_Text(t *testing.T) {
	tests := []struct {
		name string
		resp *authv1.WhoamiResponse
		want string
	}{
		{
			name: "every field",
			resp: fullWhoami,
			want: "source:      apikey\n" +
				"key:         …a1b2\n" +
				"email:       ada@example.com\n" +
				"name:        Ada\n" +
				"org:         acme\n" +
				"permissions: agent:read, agent:write\n" +
				"operator:    true\n",
		},
		{
			name: "optional rows left out",
			resp: &authv1.WhoamiResponse{Source: "basic"},
			want: "source:      basic\n" +
				"org:         \n" +
				"permissions: \n" +
				"operator:    false\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(render.EnvVar, "text")
			h := newAuthHarness(&memProfiles{})
			h.client.resp = tt.resp

			out, _, err := h.run(t, "", "whoami")
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestWhoami_JSON(t *testing.T) {
	for _, format := range []string{"json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv(render.EnvVar, format)
			h := newAuthHarness(&memProfiles{})
			h.client.resp = fullWhoami

			out, _, err := h.run(t, "", "whoami")
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &got))
			assert.Equal(t, map[string]any{
				"source": "apikey", "user_id": "u-1", "email": "ada@example.com", "name": "Ada",
				"key_hint": "a1b2", "owner_id": "u-1", "active_org_id": "o-1", "active_org_slug": "acme",
				"permissions": []any{"agent:read", "agent:write"}, "operator": true,
			}, got)
		})
	}
}
