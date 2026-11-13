package knowledge

import (
	"context"
	"errors"
	"sync"
	"time"
)

// NOTE: start blocks when full; only the round goroutine may call it, and only after checking free.
type pool struct {
	slots chan struct{}
	wg    sync.WaitGroup
}

func newPool(n int) *pool { return &pool{slots: make(chan struct{}, n)} }

func (p *pool) free() int { return cap(p.slots) - len(p.slots) }

func (p *pool) start(fn func()) {
	p.slots <- struct{}{}
	p.wg.Go(func() {
		defer func() { <-p.slots }()
		fn()
	})
}

func (p *pool) wait() { p.wg.Wait() }

// NOTE: returns only after the heartbeat ends too, so shutdown never leaves a renew in flight.
func withHeartbeat(ctx context.Context, period time.Duration, renew func(context.Context) error, job func(context.Context)) {
	ctx, cancel := context.WithCancel(ctx)
	var hb sync.WaitGroup
	defer func() {
		cancel()
		hb.Wait()
	}()
	hb.Go(func() { heartbeat(ctx, period, renew, cancel) })
	job(ctx)
}

// NOTE: one failed renew is retried on the next beat; a second in a row, or a lease already gone, cancels the job.
func heartbeat(ctx context.Context, period time.Duration, renew func(context.Context) error, lost context.CancelFunc) {
	t := time.NewTicker(period)
	defer t.Stop()
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := renew(ctx)
			switch {
			case err == nil || ctx.Err() != nil:
				failed = false
			case failed || errors.Is(err, errLeaseLost):
				lost()
				return
			default:
				failed = true
			}
		}
	}
}
