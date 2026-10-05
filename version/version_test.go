package version

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "9.9.9"

	assert.Contains(t, String(), "9.9.9")
}

func TestGet(t *testing.T) {
	assert.NotEmpty(t, Get().Version)
}

func TestDefault_FallsBackToEmbedded(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = ""

	got := Default()
	require.NotEmpty(t, got)
	assert.Equal(t, strings.TrimSpace(got), got)
}

func TestDefault_PrefersStamp(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "v1.2.3"

	assert.Equal(t, "v1.2.3", Default())
}

func TestTriple(t *testing.T) {
	tests := []struct {
		in   string
		want [3]uint32
	}{
		{"v1.2.3", [3]uint32{1, 2, 3}},
		{"0.1.0-dev", [3]uint32{0, 1, 0}},
		{"garbage", [3]uint32{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			orig := Version
			t.Cleanup(func() { Version = orig })
			Version = tt.in

			assert.Equal(t, tt.want, Triple())
		})
	}
}
