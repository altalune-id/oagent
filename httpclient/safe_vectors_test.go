package httpclient

import (
	"encoding/json"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPrivateOrLocal_SharedVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/ssrf_vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Private []string `json:"private"`
		Public  []string `json:"public"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors.Private)
	require.NotEmpty(t, vectors.Public)
	for _, s := range vectors.Private {
		assert.True(t, IsPrivateOrLocal(netip.MustParseAddr(s)), "want private: %s", s)
	}
	for _, s := range vectors.Public {
		assert.False(t, IsPrivateOrLocal(netip.MustParseAddr(s)), "want public: %s", s)
	}
}
