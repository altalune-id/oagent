package boot

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/queue"
	natsqueue "altalune.id/oagent/internal/kernel/queue/nats"
	"altalune.id/oagent/internal/testutil/natstest"
)

func TestBuildQueue_Disabled(t *testing.T) {
	q, err := buildQueue(context.Background(), kernel.Kernel{}, config.Config{}, nil, testLogger())
	require.NoError(t, err)
	_, err = q.Consumer(t.Context(), nil)
	assert.True(t, queue.IsDisabledError(err))
}

func TestBuildQueue_NATSDriverRunsOnTheSharedConnection(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	k := kernel.Kernel{Reporter: apperror.NewReporter(nil, false)}

	q, err := buildQueue(t.Context(), k, natsFeatureConfig(), nc, testLogger())
	require.NoError(t, err)
	assert.IsType(t, &natsqueue.Client{}, q)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	_, err = js.Stream(t.Context(), "WORK")
	assert.NoError(t, err, "the driver created its streams on the connection boot opened")
}

func TestBuildQueue_UnknownDriver(t *testing.T) {
	cfg := config.Config{Queue: config.QueueConfig{Enabled: true, Driver: "kafka"}}
	_, err := buildQueue(t.Context(), kernel.Kernel{}, cfg, nil, testLogger())
	assert.ErrorContains(t, err, `queue.driver "kafka"`)
}

func TestBuildQueue_NATSDriverErrorIsANilQueue(t *testing.T) {
	k := kernel.Kernel{Reporter: apperror.NewReporter(nil, false)}
	q, err := buildQueue(t.Context(), k, natsFeatureConfig(), nil, testLogger())
	require.Error(t, err)
	assert.True(t, q == nil, "no typed nil inside the interface")
}
