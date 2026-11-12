package knowledge

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const beat = 5 * time.Millisecond

type renewScript struct {
	mu    sync.Mutex
	errs  []error
	calls int
}

func (r *renewScript) renew(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if len(r.errs) == 0 {
		return nil
	}
	err := r.errs[0]
	r.errs = r.errs[1:]
	return err
}

func (r *renewScript) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func runUntil(t *testing.T, r *renewScript, calls int) (lost bool) {
	t.Helper()
	withHeartbeat(t.Context(), beat, r.renew, func(ctx context.Context) {
		deadline := time.After(2 * time.Second)
		for r.count() < calls {
			select {
			case <-ctx.Done():
				lost = true
				return
			case <-deadline:
				t.Error("the heartbeat never renewed")
				return
			case <-time.After(time.Millisecond):
			}
		}
		select {
		case <-ctx.Done():
			lost = true
		case <-time.After(10 * beat):
		}
	})
	return lost
}

func TestHeartbeat_ToleratesOneFailedRenew(t *testing.T) {
	flaky := errors.New("connection reset")
	r := &renewScript{errs: []error{flaky, nil, flaky}}
	assert.False(t, runUntil(t, r, 4), "a failed renew followed by a good one keeps the job")
}

func TestHeartbeat_TwoFailedRenewsInARowLoseTheLease(t *testing.T) {
	flaky := errors.New("connection reset")
	r := &renewScript{errs: []error{flaky, flaky}}
	assert.True(t, runUntil(t, r, 2))
}

func TestHeartbeat_LeaseGoneStopsAtOnce(t *testing.T) {
	r := &renewScript{errs: []error{errLeaseLost}}
	assert.True(t, runUntil(t, r, 1))
	assert.Equal(t, 1, r.count(), "no renew after the lease is gone")
}

func TestHeartbeat_JobReturnsOnlyAfterTheRenewEnds(t *testing.T) {
	var inFlight atomic.Int32
	started := make(chan struct{})
	renew := func(ctx context.Context) error {
		inFlight.Add(1)
		defer inFlight.Add(-1)
		select {
		case started <- struct{}{}:
		default:
		}
		time.Sleep(20 * beat)
		return nil
	}
	withHeartbeat(t.Context(), beat, renew, func(context.Context) { <-started })
	assert.Zero(t, inFlight.Load(), "shutdown waits for an in-flight renew")
}
