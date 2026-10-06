package boot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
)

func TestQueueConfig(t *testing.T) {
	o := queueConfig(config.QueueConfig{URL: "nats://x", Token: "t"})
	assert.Equal(t, "nats://x", o.URL)
	assert.Equal(t, "t", o.Token)
}

func TestBuildQueue_Disabled(t *testing.T) {
	c, err := buildQueue(context.Background(), kernel.Kernel{}, config.Config{}, testLogger())
	require.NoError(t, err)
	assert.False(t, c.Enabled())
}

func TestBuildQueue_ConnectError(t *testing.T) {
	k := kernel.Kernel{Reporter: apperror.NewReporter(nil, false)}
	cfg := config.Config{Queue: config.QueueConfig{Enabled: true, URL: "nats://127.0.0.1:1", ConnectTimeout: 50 * time.Millisecond}}

	_, err := buildQueue(context.Background(), k, cfg, testLogger())
	assert.Error(t, err)
}
