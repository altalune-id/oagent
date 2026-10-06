package schema

import (
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/migrator"
)

func TestNew_TargetVersion(t *testing.T) {
	v, err := New(migrator.Config{}).TargetVersion()
	require.NoError(t, err)
	assert.Equal(t, int64(1), v)
}

func TestMigrationsFS(t *testing.T) {
	_, err := fs.ReadFile(MigrationsFS(), "001_init.sql")
	require.NoError(t, err)
}

func TestMigrateUp_NilDB(t *testing.T) {
	assert.Error(t, MigrateUp(context.Background(), nil, migrator.Config{}))
}

func TestMigrateStatus_NilDB(t *testing.T) {
	_, err := MigrateStatus(context.Background(), nil, migrator.Config{})
	assert.Error(t, err)
}

func TestMigrateDownTo_NilDB(t *testing.T) {
	assert.Error(t, MigrateDownTo(context.Background(), nil, migrator.Config{}, 0))
}
