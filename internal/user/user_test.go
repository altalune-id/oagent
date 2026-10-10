package user

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUser_NormalizesEmail(t *testing.T) {
	u, err := New("  Alice@Example.COM ", " Alice ")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", u.Email)
	assert.Equal(t, "Alice", u.Name)
	assert.Len(t, u.PublicID, 21)
	assert.False(t, u.HasPassword())
	assert.Equal(t, u.CreatedAt, u.UpdatedAt)
}

func TestNewUser_RejectsBadEmail(t *testing.T) {
	for _, e := range []string{"", "plain", "a@", "Alice <a@example.com>", "a b@example.com", strings.Repeat("a", 250) + "@x.io"} {
		_, err := New(e, "x")
		assert.True(t, IsInvalidEmailError(err), "%q: %v", e, err)
	}
}

func TestNewUser_NameLength(t *testing.T) {
	_, err := New("a@example.com", strings.Repeat("x", nameMaxLen+1))
	assert.True(t, IsInvalidNameError(err))
	u, err := New("a@example.com", "")
	require.NoError(t, err, "empty name is allowed; install/onboarding can ask later")
	assert.Empty(t, u.Name)
}

func TestHasPassword(t *testing.T) {
	assert.True(t, (&User{PasswordHash: "$argon2id$x"}).HasPassword())
}
