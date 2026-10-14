package web

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/password"
)

func TestPasswordBundle_IsBuiltFromTheCurrentRules(t *testing.T) {
	bundle := string(builtAsset(t, "password.js"))

	for name, want := range map[string]int{"minLength": password.MinLen, "maxLength": password.MaxLen} {
		re := regexp.MustCompile(`\b` + name + `:\s*` + strconv.Itoa(want) + `\b`)
		assert.Regexp(t, re, bundle, "password.js is stale; run make web-js")
	}

	raw, err := os.ReadFile("../../password/common.txt")
	require.NoError(t, err)
	n := 0
	for line := range strings.Lines(string(raw)) {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		n++
		// The bundler inlines the list as a template literal (real newlines) or a string literal (escaped \n).
		if !strings.Contains(bundle, "\n"+entry+"\n") && !strings.Contains(bundle, `\n`+entry+`\n`) {
			t.Fatalf("password.js has no %q from password/common.txt; run make web-js", entry)
		}
	}
	assert.Positive(t, n)
}

// NOTE: make web builds these files (not committed); missing, the test skips locally and fails when CI is set.
func builtAsset(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fs.ReadFile(StaticFS(), name)
	if errors.Is(err, fs.ErrNotExist) && os.Getenv("CI") == "" {
		t.Skipf("internal/web/static/%s is not built; run make web", name)
	}
	require.NoError(t, err)
	return b
}
