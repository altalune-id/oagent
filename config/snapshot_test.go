package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_SnapshotDriverDefault(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, SnapshotDriverPostgres, c.Snapshot.Driver)
	assert.Contains(t, c.PostgresFeatures(), "snapshot.driver")
}

func TestLoad_SnapshotFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_SNAPSHOT_DRIVER", "postgres")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, SnapshotDriverPostgres, c.Snapshot.Driver)
}

func TestValidate_SnapshotDriver(t *testing.T) {
	c := validConfig(t)
	c.Snapshot.Driver = "s3"
	err := c.Validate()
	invalidSetting("snapshot.driver")(t, err)
	assert.ErrorContains(t, err, "postgres")
}
