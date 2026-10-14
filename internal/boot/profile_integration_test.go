//go:build integration

package boot

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/password"
	"altalune.id/oagent/scope"
)

func TestProfile_ThroughTheAPI(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	cookie, p, userID := f.personCookieIn(t, f.sysID, "ana@example.com", member.RoleMember)
	other, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.svcs.Sessions.Save(ctx, other, p, time.Now().Add(time.Hour)))
	client := authv1connect.NewAuthServiceClient(http.DefaultClient, f.url)
	u, err := f.svcs.Users.ByID(ctx, userID)
	require.NoError(t, err)

	got, err := client.UpdateProfile(ctx, withCookie(connect.NewRequest(&authv1.UpdateProfileRequest{Name: "Ana Maria"}), cookie))
	require.NoError(t, err)
	assert.Equal(t, u.PublicID, got.Msg.GetUserId())
	who, err := client.Whoami(ctx, withCookie(connect.NewRequest(&authv1.WhoamiRequest{}), cookie))
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", who.Msg.GetName(), "whoami shows the new name")

	_, err = client.UpdateProfile(ctx, withCookie(connect.NewRequest(&authv1.UpdateProfileRequest{Name: strings.Repeat("a", 201)}), cookie))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = client.ChangePassword(ctx, withCookie(connect.NewRequest(&authv1.ChangePasswordRequest{
		CurrentPassword: "wrong", NewPassword: "a brand new passphrase",
	}), cookie))
	assert.Equal(t, "AUT001", connectCode(t, err))
	_, err = client.ChangePassword(ctx, withCookie(connect.NewRequest(&authv1.ChangePasswordRequest{
		CurrentPassword: "quiet-forest-lamp-9", NewPassword: "ana maria rocks",
	}), cookie))
	assert.Equal(t, "AUT002", connectCode(t, err), "the new password gets the policy with the person's name as a hint")

	_, err = client.ChangePassword(ctx, withCookie(connect.NewRequest(&authv1.ChangePasswordRequest{
		CurrentPassword: "quiet-forest-lamp-9", NewPassword: "a brand new passphrase",
	}), cookie))
	require.NoError(t, err)
	u, err = f.svcs.Users.ByID(ctx, userID)
	require.NoError(t, err)
	assert.NoError(t, password.Verify(u.PasswordHash, "a brand new passphrase"))
	_, ok, err := f.svcs.Sessions.Load(ctx, other)
	require.NoError(t, err)
	assert.False(t, ok, "the other session ended")
	_, err = client.Whoami(ctx, withCookie(connect.NewRequest(&authv1.WhoamiRequest{}), cookie))
	require.NoError(t, err, "this session is kept")

	out, err := client.SignOutEverywhere(ctx, withCookie(connect.NewRequest(&authv1.SignOutEverywhereRequest{}), cookie))
	require.NoError(t, err)
	assert.True(t, out.Msg.GetSignedOut())
	_, err = client.Whoami(ctx, withCookie(connect.NewRequest(&authv1.WhoamiRequest{}), cookie))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "this session ended too")
}

func TestProfile_KeysAreRefused(t *testing.T) {
	f := newAPIFixture(t)
	client := authv1connect.NewAuthServiceClient(http.DefaultClient, f.url)
	key := f.mint(t, apikey.KindOrg, scope.AgentRead)
	_, err := client.UpdateProfile(t.Context(), withBearer(connect.NewRequest(&authv1.UpdateProfileRequest{Name: "x"}), key))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = client.ChangePassword(t.Context(), withBearer(connect.NewRequest(&authv1.ChangePasswordRequest{}), key))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = client.SignOutEverywhere(t.Context(), withBearer(connect.NewRequest(&authv1.SignOutEverywhereRequest{}), key))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}
