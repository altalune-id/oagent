package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

func TestService_UpdateProfile(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	got, err := f.svc.UpdateProfile(t.Context(), u.ID, "  Ada Lovelace ")
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", got.Name)
	_, err = f.svc.UpdateProfile(t.Context(), u.ID, strings.Repeat("a", 201))
	assert.True(t, user.IsInvalidNameError(err))
}

func TestService_ChangePassword(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	ctx := t.Context()
	for _, sid := range []string{"this", "other-1", "other-2"} {
		require.NoError(t, f.sessions.Save(ctx, sid, sessionOf(u), time.Now().Add(time.Hour)))
	}

	err := f.svc.ChangePassword(ctx, u.ID, "this", "wrong password", "a brand new passphrase")
	assert.True(t, IsInvalidCredentialsError(err), "a wrong current password is AUT001, like sign-in")
	err = f.svc.ChangePassword(ctx, u.ID, "this", testPassword, "tea-for-two")
	_, policy := errors.AsType[*password.PolicyError](err)
	assert.True(t, policy, "got %T", err)
	assert.Empty(t, f.mail.sent)

	require.NoError(t, f.svc.ChangePassword(ctx, u.ID, "this", testPassword, "a brand new passphrase"))
	assert.NoError(t, password.Verify(u.PasswordHash, "a brand new passphrase"))
	for sid, want := range map[string]bool{"this": true, "other-1": false, "other-2": false} {
		_, ok, err := f.sessions.Load(ctx, sid)
		require.NoError(t, err)
		assert.Equal(t, want, ok, sid)
	}
	require.Len(t, f.mail.sent, 1)
	assert.Equal(t, "Your oagent password was changed", f.mail.sent[0].Subject)
	assert.Equal(t, "ada@example.com", f.mail.sent[0].To)
}

func TestService_ChangePasswordWithoutAPassword(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "sso@example.com", false)
	err := f.svc.ChangePassword(t.Context(), u.ID, "", "anything", "a brand new passphrase")
	assert.True(t, IsInvalidCredentialsError(err))
}

func TestService_SignOutEverywhere(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	other := f.addUser(t, "bob@example.com", true)
	ctx := t.Context()
	require.NoError(t, f.sessions.Save(ctx, "a1", sessionOf(u), time.Now().Add(time.Hour)))
	require.NoError(t, f.sessions.Save(ctx, "a2", sessionOf(u), time.Now().Add(time.Hour)))
	require.NoError(t, f.sessions.Save(ctx, "b1", sessionOf(other), time.Now().Add(time.Hour)))
	require.NoError(t, f.svc.SignOutEverywhere(ctx, u.ID))
	for sid, want := range map[string]bool{"a1": false, "a2": false, "b1": true} {
		_, ok, err := f.sessions.Load(ctx, sid)
		require.NoError(t, err)
		assert.Equal(t, want, ok, sid)
	}
}
