package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewToken_Fixed(t *testing.T) {
	tok, err := NewToken("my-fixed-token")
	require.NoError(t, err)
	assert.Equal(t, "my-fixed-token", tok.Value())
	assert.True(t, tok.Matches("my-fixed-token"))
	assert.False(t, tok.Matches("my-fixed-token2"))
}

func TestNewToken_Generated(t *testing.T) {
	a, err := NewToken("")
	require.NoError(t, err)
	b, err := NewToken("")
	require.NoError(t, err)
	assert.Len(t, a.Value(), 32)
	assert.NotEqual(t, a.Value(), b.Value())
	assert.True(t, a.Matches(a.Value()))
	assert.False(t, a.Matches(b.Value()))
	assert.False(t, a.Matches(""))
}

func TestToken_ZeroNeverMatches(t *testing.T) {
	assert.False(t, Token{}.Matches(""))
}
