package mcpserver

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReservedHeaders_SharedList(t *testing.T) {
	raw, err := os.ReadFile("testdata/reserved_headers.json")
	require.NoError(t, err)
	var shared []string
	require.NoError(t, json.Unmarshal(raw, &shared))
	assert.Equal(t, shared, ReservedHeaders)
}
