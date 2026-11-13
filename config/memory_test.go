package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MemoryDefaults(t *testing.T) {
	c := validConfig(t)
	assert.True(t, c.Memory.Native.Enabled, "the native backend needs only Postgres; each agent still opts in by picking a store")
	assert.Equal(t, []string{MemoryBackendNative}, c.Memory.Backends())
}

func TestLoad_MemoryNativeOffFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_MEMORY_NATIVE_ENABLED", "false")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.False(t, c.Memory.Native.Enabled)
	assert.Empty(t, c.Memory.Backends(), "no backend on means no store can be created and no run remembers")
}
