package run_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The run domain reaches other domains only through its own ports, so it builds without any of them.
func TestRun_DependsOnNoOtherDomain(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	require.NoError(t, err)
	for pkg := range strings.FieldsSeq(string(out)) {
		rest, ok := strings.CutPrefix(pkg, "altalune.id/oagent/internal/")
		if !ok || rest == "run" || strings.HasPrefix(rest, "kernel/") {
			continue
		}
		assert.Fail(t, "internal/run depends on another domain", pkg)
	}
}
