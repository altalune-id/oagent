package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatalog(t *testing.T) {
	assert.Contains(t, Names(), "org:read")
	assert.Contains(t, Names(), "agent:write")
	assert.True(t, Valid("agent:read"))
	assert.False(t, Valid("nope"))

	level, ok := LevelOf("org:write")
	assert.True(t, ok)
	assert.Equal(t, LevelOrg, level)

	level, ok = LevelOf("agent:read")
	assert.True(t, ok)
	assert.Equal(t, LevelProject, level)

	_, ok = LevelOf("nope")
	assert.False(t, ok)

	assert.Len(t, All(), len(Names()))
}

func TestEverySpecHasLevel(t *testing.T) {
	for _, s := range All() {
		assert.Contains(t, []Level{LevelOrg, LevelProject}, s.Level, s.Name)
	}
}
