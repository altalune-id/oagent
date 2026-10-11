package user_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/user"
)

type setPasswordCall struct {
	email, password string
	calls           int
}

func runSetPassword(t *testing.T, fail error, stdin string, args ...string) (string, *setPasswordCall, error) {
	t.Helper()
	got := &setPasswordCall{}
	cmd := user.NewCmd(func(_ context.Context, email, password string) error {
		got.email, got.password = email, password
		got.calls++
		return fail
	})
	buf := new(bytes.Buffer)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"set-password"}, args...))
	err := cmd.ExecuteContext(t.Context())
	return buf.String(), got, err
}

func TestSetPassword_FromStdin(t *testing.T) {
	out, got, err := runSetPassword(t, nil, "a brand new password\n", "--email", "ada@example.com", "--password-stdin")
	require.NoError(t, err)
	assert.Equal(t, "ada@example.com", got.email)
	assert.Equal(t, "a brand new password", got.password)
	assert.Contains(t, out, "Password updated for ada@example.com; their sessions were signed out.")
	assert.NotContains(t, out, "a brand new password")
}

func TestSetPassword_PipedStdinWithoutFlag(t *testing.T) {
	_, got, err := runSetPassword(t, nil, "a brand new password\n", "--email", "ada@example.com")
	require.NoError(t, err)
	assert.Equal(t, "a brand new password", got.password)
}

func TestSetPassword_RequiresEmail(t *testing.T) {
	_, got, err := runSetPassword(t, nil, "a brand new password\n", "--password-stdin")
	require.Error(t, err)
	assert.Zero(t, got.calls)
}

func TestSetPassword_EmptyInputIsAnError(t *testing.T) {
	_, got, err := runSetPassword(t, nil, "", "--email", "ada@example.com", "--password-stdin")
	require.ErrorContains(t, err, "set-password: read password")
	assert.Zero(t, got.calls)
}

func TestSetPassword_Error(t *testing.T) {
	out, _, err := runSetPassword(t, &user.NotFoundError{}, "a brand new password\n", "--email", "nobody@example.com", "--password-stdin")
	require.Error(t, err)
	assert.True(t, user.IsNotFoundError(err))
	assert.NotContains(t, out, "Password updated")
}
