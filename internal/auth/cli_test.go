package auth_test

import (
	"bytes"
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/auth"
)

type memProfiles struct {
	s     *auth.CLISession
	saved int
}

func (m *memProfiles) Target(*cobra.Command) (string, string, error) {
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
}

func (c whoamiOnly) Whoami(context.Context, *connect.Request[authv1.WhoamiRequest]) (*connect.Response[authv1.WhoamiResponse], error) {
	return connect.NewResponse(c.resp), nil
}

func runAuth(t *testing.T, profiles *memProfiles, fromProfile bool, args ...string) string {
	t.Helper()
	client := whoamiOnly{resp: &authv1.WhoamiResponse{Source: "local", Email: "ada@example.com", ActiveOrgSlug: "acme"}}
	cmd := auth.NewCmd(
		profiles,
		func(*cobra.Command) (authv1connect.AuthServiceClient, bool, error) { return client, fromProfile, nil },
		func(*cobra.Command, string, string) (authv1connect.AuthServiceClient, error) { return client, nil },
	)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	return buf.String()
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

func TestLogout_NotSignedIn(t *testing.T) {
	out := runAuth(t, &memProfiles{}, false, "logout")
	assert.Contains(t, out, "not signed in (profile dev)")
}
