package web

import (
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
	js, err := fs.ReadFile(StaticFS(), "password.js")
	require.NoError(t, err)
	bundle := string(js)

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
		if !strings.Contains(bundle, "\n"+entry+"\n") {
			t.Fatalf("password.js has no %q from password/common.txt; run make web-js", entry)
		}
	}
	assert.Positive(t, n)
}
