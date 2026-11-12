package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_KnowledgeDefaults(t *testing.T) {
	c := validConfig(t)
	assert.False(t, c.Knowledge.Pgvector.Enabled)
	assert.Empty(t, c.Knowledge.Backends(), "knowledge is off until a backend is enabled")
	assert.False(t, c.Knowledge.AllowPrivateHosts, "production reaches only public hosts unless the operator says so")
}

func TestLoad_KnowledgeFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_KNOWLEDGE_PGVECTOR_ENABLED", "true")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.Knowledge.Pgvector.Enabled)
	assert.Equal(t, []string{KnowledgeBackendPgvector}, c.Knowledge.Backends())

	t.Setenv("OAGENT_KNOWLEDGE_ALLOW_PRIVATE_HOSTS", "true")
	c, err = Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.Knowledge.AllowPrivateHosts)
}
