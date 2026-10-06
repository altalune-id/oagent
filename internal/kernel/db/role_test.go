package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/pg"
)

func TestIsInvalidRoleError(t *testing.T) {
	assert.True(t, IsInvalidRoleError(pg.ValidateIdentifier("")))
	assert.True(t, IsInvalidRoleError(pg.ValidateIdentifier("own\ner")))
	assert.True(t, IsInvalidRoleError(pg.ValidateIdentifier(strings.Repeat("a", 64))))
	assert.False(t, IsInvalidRoleError(nil))
	assert.False(t, IsInvalidRoleError(assert.AnError))
}
