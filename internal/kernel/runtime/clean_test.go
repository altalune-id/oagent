package runtime_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/runtime"
)

func TestClean(t *testing.T) {
	in := []string{"  ab  ", "", "   ", strings.Repeat("é", 6) + " ", "x", "y"}
	assert.Equal(t, []string{"ab", "éééé", "x"}, runtime.Clean(in, 4, 3), "trimmed, blanks dropped, cut by runes, capped")
	assert.Empty(t, runtime.Clean(nil, 4, 3))
	assert.Equal(t, []string{"ab"}, runtime.Clean([]string{"ab c"}, 3, 1), "a cut never leaves trailing space")
}
