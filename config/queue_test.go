package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_QueueDefaults(t *testing.T) {
	c := validConfig(t)
	assert.False(t, c.Queue.Enabled)
	assert.Equal(t, QueueDriverNATS, c.Queue.Driver)
}

func TestLoad_QueueFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_QUEUE_ENABLED", "true")
	t.Setenv("OAGENT_QUEUE_DRIVER", "nats")
	t.Setenv("OAGENT_NATS_URL", "nats://127.0.0.1:4222")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.Queue.Enabled)
	assert.Equal(t, QueueDriverNATS, c.Queue.Driver)
}

func TestValidate_QueueDriver(t *testing.T) {
	c := validConfig(t)
	c.Queue.Driver = "kafka"
	err := c.Validate()
	invalidSetting("queue.driver")(t, err)
	assert.ErrorContains(t, err, "nats")
}
