package db

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/stretchr/testify/assert"
)

func TestIsNoRows(t *testing.T) {
	assert.True(t, IsNoRows(qrm.ErrNoRows))
	assert.True(t, IsNoRows(sql.ErrNoRows))
	assert.True(t, IsNoRows(fmt.Errorf("load: %w", qrm.ErrNoRows)))
	assert.False(t, IsNoRows(errors.New("boom")))
	assert.False(t, IsNoRows(nil))
}
