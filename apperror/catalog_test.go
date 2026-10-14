package apperror

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type catalogEntry struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Params  []string `json:"params"`
	Reasons []string `json:"reasons"`
}

func loadCatalog(t *testing.T) []catalogEntry {
	t.Helper()
	raw, err := os.ReadFile("codes.json")
	require.NoError(t, err)
	var entries []catalogEntry
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.NotEmpty(t, entries)
	return entries
}

func TestCatalog_EveryCodeHasItsKind(t *testing.T) {
	byName := map[string]Kind{
		"invalid":             KindInvalid,
		"not_found":           KindNotFound,
		"already_exists":      KindAlreadyExists,
		"unauthenticated":     KindUnauthenticated,
		"forbidden":           KindForbidden,
		"failed_precondition": KindFailedPrecondition,
		"internal":            KindInternal,
		"resource_exhausted":  KindResourceExhausted,
	}
	entries := loadCatalog(t)
	for _, e := range entries {
		want, ok := byName[e.Kind]
		require.True(t, ok, "%s: unknown kind %q", e.Code, e.Kind)
		got, ok := kinds[Code(e.Code)]
		assert.True(t, ok, "%s has no kind; run make generate", e.Code)
		assert.Equal(t, want, got, e.Code)
	}
	assert.Len(t, kinds, len(entries), "kind table and codes.json differ; run make generate")
}

func TestCatalog_KeepsEveryIssuedCode(t *testing.T) {
	raw, err := os.ReadFile("issued_codes.txt")
	require.NoError(t, err)
	issued := map[string]string{}
	for line := range strings.Lines(string(raw)) {
		code, name, ok := strings.Cut(strings.TrimSpace(line), " ")
		require.True(t, ok, "issued_codes.txt: %q is not \"CODE Name\"", line)
		issued[code] = name
	}
	inCatalog := map[string]string{}
	for _, e := range loadCatalog(t) {
		inCatalog[e.Code] = e.Name
		name, ok := issued[e.Code]
		assert.True(t, ok, "%s is new: append \"%s %s\" to apperror/issued_codes.txt", e.Code, e.Code, e.Name)
		if ok {
			assert.Equal(t, name, e.Name, "%s was renamed; codes are append-only", e.Code)
		}
	}
	for code := range issued {
		assert.Contains(t, inCatalog, code, "%s was removed from codes.json; codes are append-only", code)
	}
}
