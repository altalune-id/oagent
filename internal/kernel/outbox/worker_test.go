package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
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

type dueEntry struct {
	e   Entry
	due time.Time
}

type queueStore struct {
	Store
	mu          sync.Mutex
	due         map[uuid.UUID]*dueEntry
	claims      int
	delivered   int
	claimErrs   int
	succeedErrs int
}

func (q *queueStore) add(e Entry, due time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.due == nil {
		q.due = map[uuid.UUID]*dueEntry{}
	}
	q.due[e.ID] = &dueEntry{e: e, due: due}
}

func (q *queueStore) ClaimDue(_ context.Context, now time.Time, limit int) ([]Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims++
	if q.claimErrs > 0 {
		q.claimErrs--
		return nil, errors.New("cold start")
	}
	var out []Entry
	for _, d := range q.due {
		if len(out) < limit && !d.due.After(now) {
			d.e.Attempt++
			d.due = now.Add(ClaimLease)
			out = append(out, d.e)
		}
	}
	return out, nil
}

func (q *queueStore) Succeed(_ context.Context, e Entry, _ time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.succeedErrs > 0 {
		q.succeedErrs--
		return errors.New("cold start")
	}
	delete(q.due, e.ID)
	q.delivered++
	return nil
}

func (q *queueStore) Fail(_ context.Context, e Entry, retryAt time.Time, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.due[e.ID].due = retryAt
	return nil
}

func (q *queueStore) counts() (claims, delivered int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims, q.delivered
}

type flakyDeliverer struct {
	mu    sync.Mutex
	fails int
}

func (f *flakyDeliverer) Deliver(context.Context, Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return errors.New("nope")
	}
	return nil
}

func idleOpts() WorkerOpts {
	return WorkerOpts{Tick: 5 * time.Second, IdlePoll: 10 * time.Minute}
}

func runWorker(t *testing.T, w *Worker) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	synctest.Wait()
}

func TestWorkerRunSweepsAtStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		store.add(claimedEntry(), time.Now())
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts()))

		claims, delivered := store.counts()
		assert.Equal(t, 1, claims)
		assert.Equal(t, 1, delivered)
	})
}

func TestWorkerWakeDeliversBeforeIdlePoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		w := NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts())
		runWorker(t, w)

		store.add(claimedEntry(), time.Now())
		w.Wake()
		synctest.Wait()

		_, delivered := store.counts()
		assert.Equal(t, 1, delivered, "a wake sweeps at once, without waiting for the idle poll")
	})
}

func TestWorkerWakeNeverBlocks(t *testing.T) {
	w := NewWorker(&queueStore{}, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts())
	for range 3 {
		w.Wake()
	}
}

func TestWorkerIdlePollCatchesAMissedWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts()))

		store.add(claimedEntry(), time.Now())
		time.Sleep(10*time.Minute - time.Second)
		synctest.Wait()
		_, delivered := store.counts()
		assert.Zero(t, delivered, "nothing sweeps before the idle poll")

		time.Sleep(2 * time.Second)
		synctest.Wait()
		_, delivered = store.counts()
		assert.Equal(t, 1, delivered)
	})
}

func TestWorkerRetriesBeforeIdlePoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		store.add(claimedEntry(), time.Now())
		runWorker(t, NewWorker(store, &flakyDeliverer{fails: 1}, &fakeTenants{}, nil, idleOpts()))

		_, delivered := store.counts()
		require.Zero(t, delivered, "the first delivery fails")

		time.Sleep(BackoffBase(2) + BackoffBase(2)/10 + time.Second)
		synctest.Wait()
		_, delivered = store.counts()
		assert.Equal(t, 1, delivered, "the retry runs at its next attempt time, not at the idle poll")
	})
}

func TestWorkerSweepsAgainOneTickAfterAFullBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		store.add(claimedEntry(), time.Now())
		store.add(claimedEntry(), time.Now())
		opts := idleOpts()
		opts.Batch = 1
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, opts))

		_, delivered := store.counts()
		require.Equal(t, 1, delivered, "a full batch waits a tick before the next sweep")
		time.Sleep(opts.Tick)
		synctest.Wait()
		_, delivered = store.counts()
		assert.Equal(t, 2, delivered)
	})
}

func TestWorkerRetriesOneTickAfterAClaimError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{claimErrs: 1}
		store.add(claimedEntry(), time.Now())
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts()))

		time.Sleep(idleOpts().Tick)
		synctest.Wait()
		_, delivered := store.counts()
		assert.Equal(t, 1, delivered, "a failed claim is tried again after a tick, not at the idle poll")
	})
}

type flakyTenants struct {
	mu   sync.Mutex
	errs int
}

func (f *flakyTenants) Each(ctx context.Context, fn func(context.Context, string) error) error {
	f.mu.Lock()
	failing := f.errs > 0
	if failing {
		f.errs--
	}
	f.mu.Unlock()
	if failing {
		return errors.New("cold start")
	}
	return fn(ctx, "org")
}

func TestWorkerRetriesOneTickAfterATenantError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		store.add(claimedEntry(), time.Now())
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &flakyTenants{errs: 1}, nil, idleOpts()))

		time.Sleep(idleOpts().Tick)
		synctest.Wait()
		_, delivered := store.counts()
		assert.Equal(t, 1, delivered, "a failed enumeration is tried again after a tick")
	})
}

func TestWorkerRetriesAtTheLeaseAfterASettleError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{succeedErrs: 1}
		store.add(claimedEntry(), time.Now())
		runWorker(t, NewWorker(store, &flakyDeliverer{}, &fakeTenants{}, nil, idleOpts()))

		time.Sleep(ClaimLease + time.Second)
		synctest.Wait()
		_, delivered := store.counts()
		assert.Equal(t, 1, delivered, "an unsettled claim is swept again when its lease ends, not at the idle poll")
	})
}

func TestWorkerTracksEveryRetryTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &queueStore{}
		early, late := claimedEntry(), claimedEntry()
		late.Attempt = 2
		store.add(early, time.Now())
		store.add(late, time.Now())
		opts := idleOpts()
		opts.IdlePoll = 24 * time.Hour
		runWorker(t, NewWorker(store, &flakyDeliverer{fails: 2}, &fakeTenants{}, nil, opts))

		time.Sleep(BackoffBase(2) + BackoffBase(2)/10 + time.Second)
		synctest.Wait()
		_, delivered := store.counts()
		require.Equal(t, 1, delivered, "the earlier retry runs first")
		time.Sleep(BackoffBase(4) + BackoffBase(4)/10)
		synctest.Wait()
		_, delivered = store.counts()
		assert.Equal(t, 2, delivered, "the later retry is not lost once the earlier one ran")
	})
}
