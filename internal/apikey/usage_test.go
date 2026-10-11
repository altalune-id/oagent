package apikey_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestUsageWorker_KeepsLatestAndFlushesOnTick(t *testing.T) {
	store := &fakes.APIKeyStore{}
	w := apikey.NewUsageWorker(store, 10*time.Millisecond, nil)
	id := uuid.New()
	t0 := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	w.Record(id, t0.Add(time.Minute))
	w.Record(id, t0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	require.Eventually(t, func() bool { return len(store.Batches()) == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)

	batches := store.Batches()
	require.Len(t, batches, 1)
	assert.True(t, t0.Add(time.Minute).Equal(batches[0][id]))
}

func TestUsageWorker_DrainsOnShutdown(t *testing.T) {
	store := &fakes.APIKeyStore{}
	w := apikey.NewUsageWorker(store, time.Hour, nil)
	id := uuid.New()
	w.Record(id, time.Now())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.Run(ctx), context.Canceled)
	require.Len(t, store.Batches(), 1)
	assert.Contains(t, store.Batches()[0], id)
}

func TestUsageWorker_NothingPendingWritesNothing(t *testing.T) {
	store := &fakes.APIKeyStore{}
	w := apikey.NewUsageWorker(store, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = w.Run(ctx)
	assert.Empty(t, store.Batches())
	assert.Equal(t, "apikey-usage", w.Name())
}
