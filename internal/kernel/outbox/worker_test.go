package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	Store
	claimed   []Entry
	claimErr  error
	succeeded []Entry
	failed    []Entry
}

func (f *fakeStore) ClaimDue(context.Context, time.Time, int) ([]Entry, error) {
	return f.claimed, f.claimErr
}

func (f *fakeStore) Succeed(_ context.Context, e Entry, _ time.Time) error {
	f.succeeded = append(f.succeeded, e)
	return nil
}

func (f *fakeStore) Fail(_ context.Context, e Entry, _ time.Time, _ string) error {
	f.failed = append(f.failed, e)
	return nil
}

type fakeDeliverer struct {
	err    error
	called int
}

func (f *fakeDeliverer) Deliver(context.Context, Entry) error {
	f.called++
	return f.err
}

type fakeTenants struct {
	err   error
	calls int
}

func (f *fakeTenants) Each(ctx context.Context, fn func(context.Context, string) error) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return fn(ctx, "org")
}

func claimedEntry() Entry {
	return Entry{ID: uuid.New(), EventID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New(), Target: "ep"}
}

func TestWorkerIdleWhenUnconfigured(t *testing.T) {
	assert.NoError(t, NewWorker(nil, nil, nil, nil, WorkerOpts{}).Run(context.Background()))
}

func TestWorkerSweep(t *testing.T) {
	store := &fakeStore{claimed: []Entry{claimedEntry()}}
	deliver := &fakeDeliverer{}
	tenants := &fakeTenants{}
	w := NewWorker(store, deliver, tenants, nil, WorkerOpts{})

	w.sweep(context.Background())
	assert.Equal(t, 1, tenants.calls)
	assert.Equal(t, 1, deliver.called)
	assert.Len(t, store.succeeded, 1)
	assert.Empty(t, store.failed)
}

func TestWorkerSweepFail(t *testing.T) {
	store := &fakeStore{claimed: []Entry{claimedEntry()}}
	deliver := &fakeDeliverer{err: errors.New("nope")}
	w := NewWorker(store, deliver, &fakeTenants{}, nil, WorkerOpts{})

	w.sweep(context.Background())
	assert.Empty(t, store.succeeded)
	require.Len(t, store.failed, 1)
}

func TestWorkerSweepClaimError(t *testing.T) {
	store := &fakeStore{claimErr: errors.New("boom")}
	deliver := &fakeDeliverer{}
	w := NewWorker(store, deliver, &fakeTenants{}, nil, WorkerOpts{})

	w.sweep(context.Background())
	assert.Zero(t, deliver.called)
}

func TestWorkerSweepTenantError(t *testing.T) {
	w := NewWorker(&fakeStore{}, &fakeDeliverer{}, &fakeTenants{err: errors.New("boom")}, nil, WorkerOpts{})
	// must not panic; logs the enumerate error.
	assert.NotPanics(t, func() { w.sweep(context.Background()) })
}

func TestWorkerName(t *testing.T) {
	assert.Equal(t, WorkerName, NewWorker(nil, nil, nil, nil, WorkerOpts{}).Name())
}

func TestWorkerRunTicksThenStops(t *testing.T) {
	store := &fakeStore{}
	w := NewWorker(store, &fakeDeliverer{}, &fakeTenants{}, nil, WorkerOpts{Tick: time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.NoError(t, w.Run(ctx))
}
