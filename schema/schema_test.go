package schema

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/migrator"
)

func TestNew_TargetVersion(t *testing.T) {
	names, err := fs.Glob(MigrationsFS(), "*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, names)
	var latest int64
	for _, name := range names {
		num, _, ok := strings.Cut(name, "_")
		require.True(t, ok, name)
		n, err := strconv.ParseInt(num, 10, 64)
		require.NoError(t, err, name)
		latest = max(latest, n)
	}

	v, err := New(migrator.Config{}).TargetVersion()
	require.NoError(t, err)
	assert.Equal(t, latest, v, "VERSION must name the newest migration")
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
