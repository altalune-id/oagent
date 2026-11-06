package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStore_NeverWritesSnapshots(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, banned := range []string{"ThreadSnapshots", "thread_snapshots"} {
			assert.NotContains(t, string(src), banned, "%s: thread_snapshots has one owner, the SnapshotStore seam", f)
		}
	}
}
