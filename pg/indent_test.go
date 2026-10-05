package pg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIdentifier(t *testing.T) {
	assert.NoError(t, ValidateIdentifier("owner"))
	assert.True(t, IsInvalidIdentifierError(ValidateIdentifier("")))
	assert.True(t, IsInvalidIdentifierError(ValidateIdentifier("own\ner")))
	assert.True(t, IsInvalidIdentifierError(ValidateIdentifier(strings.Repeat("a", 64))))
}

func TestInvalidIdentifierError(t *testing.T) {
	err := ValidateIdentifier("")
	assert.Contains(t, err.Error(), "empty")
	assert.True(t, IsInvalidIdentifierError(err))
	assert.False(t, IsInvalidIdentifierError(nil))
	assert.False(t, IsInvalidIdentifierError(assert.AnError))
}

func TestQuoteIdentifier(t *testing.T) {
	assert.Equal(t, `"ow""ner"`, QuoteIdentifier(`ow"ner`))
}
