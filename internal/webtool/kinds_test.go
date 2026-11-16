package webtool

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableKinds_MatchSharedJSON(t *testing.T) {
	raw, err := os.ReadFile("testdata/search_kinds.json")
	require.NoError(t, err)
	var shared []Kind
	require.NoError(t, json.Unmarshal(raw, &shared))
	assert.Equal(t, shared, AvailableKinds())
}
