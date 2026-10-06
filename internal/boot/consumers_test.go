package boot

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/config"
)

func TestConsumerOff(t *testing.T) {
	enabled := config.Config{Queue: config.QueueConfig{Enabled: true}}

	assert.Equal(t, "", consumerOff(newOptions(), enabled))
	assert.Equal(t, "WithConsumer(false)", consumerOff(&options{consumer: false}, enabled))
	assert.Equal(t, "WithSchedulerOnly", consumerOff(&options{consumer: true, schedulerOnly: true}, enabled))
	assert.Equal(t, "queue.enabled=false", consumerOff(newOptions(), config.Config{}))
}

func TestWithConsumer(t *testing.T) {
	o := newOptions()
	assert.True(t, o.consumer)

	WithConsumer(false)(o)
	assert.False(t, o.consumer)
	WithConsumerOnly(true)(o)
	assert.True(t, o.consumerOnly)
}

func TestQueueHandlersAndListeners(t *testing.T) {
	assert.Nil(t, queueHandlers(nil))
	assert.Nil(t, queueListeners(nil))
}
