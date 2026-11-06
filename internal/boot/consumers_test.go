package boot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/run"
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
	assert.Nil(t, queueHandlers(&Services{}), "no runner, no handler")
	runner := run.NewRunner(nil, nil, run.RunnerOptions{})
	hs := queueHandlers(&Services{RunTurns: runner})
	require.Len(t, hs, 1)
	assert.Equal(t, run.TurnJob, hs[0].Job)
	assert.Equal(t, []queue.Job{run.TurnJob}, queueJobs(), "every handled job is declared")
	assert.Nil(t, queueListeners(nil))
}
