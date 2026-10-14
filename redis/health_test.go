package redis

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review Focus 3: a Redis that stops answering makes the server not ready, and it never crashes the worker.
func TestHealth_ProbeFailureMarksNotReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
		require.NoError(t, client.Close())
		var logs bytes.Buffer
		rh := NewHealth(client, time.Second, slog.New(slog.NewTextHandler(&logs, nil)))
		require.True(t, rh.Ready())

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- rh.Run(ctx) }()
		synctest.Wait()
		assert.False(t, rh.Ready())
		time.Sleep(3 * probeEvery)
		synctest.Wait()
		assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("redis: not ready")), "logged once per change, not per probe")

		cancel()
		assert.NoError(t, <-done)
	})
}
