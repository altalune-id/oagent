package migrator

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://user:pass@localhost:5432/db"

func TestOpen_NoRole(t *testing.T) {
	db, err := Open(context.Background(), testDSN, Config{})
	require.NoError(t, err)
	require.NotNil(t, db)
	assert.Equal(t, 2, db.Stats().MaxOpenConnections)
	assert.NoError(t, db.Close())
}

func TestOpen_WithRole(t *testing.T) {
	db, err := Open(context.Background(), testDSN, Config{Role: "owner"})
	require.NoError(t, err)
	assert.NoError(t, db.Close())
}

func TestOpen_InvalidDSN(t *testing.T) {
	_, err := Open(context.Background(), "postgres://user:pass@host:notaport/db", Config{})
	assert.Error(t, err)
}

func TestOpen_InvalidRole(t *testing.T) {
	_, err := Open(context.Background(), testDSN, Config{Role: strings.Repeat("a", 64)})
	assert.Error(t, err)
}
