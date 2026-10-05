package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	for _, format := range []string{"text", "json", ""} {
		l, err := newLogger("debug", format, new(bytes.Buffer))
		require.NoError(t, err)
		assert.NotNil(t, l)
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, err := newLogger("bogus", "text", new(bytes.Buffer))
	assert.Error(t, err)
}

func TestNewLogger_InvalidFormat(t *testing.T) {
	_, err := newLogger("info", "xml", new(bytes.Buffer))
	assert.Error(t, err)
}
