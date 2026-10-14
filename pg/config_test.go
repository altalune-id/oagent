package pg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDBConfig_Validate(t *testing.T) {
	assert.NoError(t, Config{DSN: "postgres://x"}.Validate())
	assert.Error(t, Config{}.Validate())
}
