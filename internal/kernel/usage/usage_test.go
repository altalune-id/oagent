package usage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
)

type fakeToucher struct {
	mu      sync.Mutex
	batches []map[uuid.UUID]time.Time
	scopes  []tenant.Context
	err     error
}

func (f *fakeToucher) TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	scope, _ := tenant.From(ctx)
	f.scopes = append(f.scopes, scope)
	f.batches = append(f.batches, used)
	return f.err
}

func (f *fakeToucher) seen() ([]map[uuid.UUID]time.Time, []tenant.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[uuid.UUID]time.Time(nil), f.batches...), append([]tenant.Context(nil), f.scopes...)
}

func TestWorker_KeepsLatestAndFlushesOnTick(t *testing.T) {
	store := &fakeToucher{}
	w := New("test-usage", store, 10*time.Millisecond, nil)
	id, scope := uuid.New(), tenant.Context{OrgID: uuid.New()}
	t0 := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	w.Record(scope, id, t0.Add(time.Minute))
	w.Record(scope, id, t0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	require.Eventually(t, func() bool { b, _ := store.seen(); return len(b) == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	batches, _ := store.seen()
	require.Len(t, batches, 1)
	assert.True(t, t0.Add(time.Minute).Equal(batches[0][id]), "the latest time wins")
}

func TestWorker_DrainsOnShutdown(t *testing.T) {
	store := &fakeToucher{}
	w := New("test-usage", store, time.Hour, nil)
	id := uuid.New()
	w.Record(tenant.Context{OrgID: uuid.New()}, id, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.Run(ctx), context.Canceled)
	batches, _ := store.seen()
	require.Len(t, batches, 1)
	assert.Contains(t, batches[0], id)
}

func TestWorker_NothingPendingWritesNothing(t *testing.T) {
	store := &fakeToucher{}
	w := New("apikey-usage", store, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = w.Run(ctx)
	batches, _ := store.seen()
	assert.Empty(t, batches)
	assert.Equal(t, "apikey-usage", w.Name())
}

// SECURITY: tables are scoped under row level security, so each tenant's records are flushed in that tenant's scope.
func TestWorker_FlushesEachTenantInItsOwnScope(t *testing.T) {
	store := &fakeToucher{}
	w := New("test-usage", store, time.Hour, nil)
	a := tenant.Context{OrgID: uuid.New()}
	b := tenant.Context{OrgID: uuid.New(), ProjectID: uuid.New()}
	keyA, keyB := uuid.New(), uuid.New()
	w.Record(a, keyA, time.Now())
	w.Record(b, keyB, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.Run(ctx), context.Canceled)
	batches, scopes := store.seen()
	require.Len(t, batches, 2)
	for i, scope := range scopes {
		want := map[tenant.Context]uuid.UUID{a: keyA, b: keyB}[scope]
		assert.Contains(t, batches[i], want, "scope %v", scope)
		assert.Len(t, batches[i], 1)
	}
	assert.ElementsMatch(t, []tenant.Context{a, b}, scopes)
}

func TestWorker_FlushErrorIsLoggedNotFatal(t *testing.T) {
	store := &fakeToucher{err: errors.New("db down")}
	w := New("test-usage", store, time.Hour, nil)
	w.Record(tenant.Context{OrgID: uuid.New()}, uuid.New(), time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.Run(ctx), context.Canceled)
	batches, _ := store.seen()
	assert.Len(t, batches, 1)
}
