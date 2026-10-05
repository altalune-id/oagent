package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDBConfig_Validate(t *testing.T) {
	assert.NoError(t, DBConfig{DSN: "postgres://x"}.Validate())
	assert.Error(t, DBConfig{}.Validate())
}
