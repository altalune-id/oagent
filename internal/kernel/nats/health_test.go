package nats

import (
	"bytes"
	"context"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/natstest"
)

func TestHealth_FollowsTheConnection(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := natsgo.Connect(ns.ClientURL(), natsgo.MaxReconnects(-1), natsgo.ReconnectWait(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	log, logs := bufferLogger()
	nh := NewHealth(nc, log)

	nh.probe(t.Context())
	assert.True(t, nh.Ready())

	ns.Shutdown()
	require.Eventually(t, func() bool { return !nc.IsConnected() }, 5*time.Second, 10*time.Millisecond)
	nh.probe(t.Context())
	nh.probe(t.Context())
	assert.False(t, nh.Ready())
	assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("nats: not ready")), "logged once per change, not per probe")

	nc.Close()
	nh.probe(t.Context())
	assert.False(t, nh.Ready(), "a closed connection is not ready")
}

func TestHealth_ReadyAgainIsLogged(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := natsgo.Connect(ns.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	log, logs := bufferLogger()
	nh := NewHealth(nc, log)
	nh.ready.Store(false)

	nh.probe(t.Context())
	assert.True(t, nh.Ready())
	assert.Contains(t, logs.String(), "nats: ready again")
}

func TestHealth_RunStopsWithTheContext(t *testing.T) {
	ns := natstest.Start(t)
	nc, err := natsgo.Connect(ns.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	nh := NewHealth(nc, discard())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- nh.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}
