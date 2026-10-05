package nanoid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Length(t *testing.T) {
	for _, n := range []int{8, 16, 21, 32} {
		s, err := New(n)
		require.NoError(t, err)
		assert.Len(t, s, n)
	}
}

func TestNew_Alphabet(t *testing.T) {
	s, err := New(64)
	require.NoError(t, err)

	const allowed = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789-_"
	const ambiguous = "0Oo1IiLl"
	for _, r := range s {
		assert.True(t, strings.ContainsRune(allowed, r), "unexpected character %q", r)
		assert.False(t, strings.ContainsRune(ambiguous, r), "ambiguous character %q", r)
	}
}

func TestNew_ZeroLength(t *testing.T) {
	_, err := New(0)
	assert.Error(t, err)
}

func TestMustNew(t *testing.T) {
	assert.Len(t, MustNew(21), 21)
	assert.Panics(t, func() { MustNew(0) })
}

func TestNewInviteToken_Deterministic(t *testing.T) {
	raw, hash, err := NewInviteToken()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(raw), 21)
	assert.Len(t, hash, 64)
}

func TestNewInviteToken_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := range 1000 {
		raw, _, err := NewInviteToken()
		require.NoError(t, err)
		assert.False(t, seen[raw], "collision at iter %d", i)
		seen[raw] = true
	}
}

func TestIsValid(t *testing.T) {
	s, err := New(21)
	require.NoError(t, err)

	assert.True(t, IsValid(s, 21))
	assert.False(t, IsValid(s, 22))
	for _, bad := range []string{"", "abc0", "iiiiiiiiiiiiiiiiiiiii", "000000000000000000000"} {
		assert.False(t, IsValid(bad, len(bad)), "IsValid(%q)", bad)
	}
}
