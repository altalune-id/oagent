package run_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func runnerOpts() run.RunnerOptions {
	return run.RunnerOptions{
		MaxConcurrent: 8, Buffer: 16, LeaseTTL: time.Minute, Heartbeat: 20 * time.Second,
		DrainTimeout: 25 * time.Second, TurnTimeout: 10 * time.Minute,
	}
}

func startRunner(t *testing.T, r *run.Runner) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Hour):
			t.Fatal("Run never returned")
		}
	}
}

func (f *svcFixture) kick() run.Kick { return run.Kick{OrgID: f.org, ThreadID: f.thread} }

func (f *svcFixture) waitRun(t *testing.T, ok func(*run.Run) bool) *run.Run {
	t.Helper()
	for range 200 {
		synctest.Wait()
		for _, r := range f.store.AllRuns() {
			if ok(&r) {
				return &r
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("no run reached the wanted state")
	return nil
}

func TestTurnRunner_BurstCoalesces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		for _, text := range []string{"one", "two", "three"} {
			f.say(t, text)
		}
		f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		r := run.NewRunner(f.svc, f.store, runnerOpts())
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusCompleted })
		stop()
		require.Len(t, f.store.AllRuns(), 1, "three pending messages make one run")
		require.Len(t, f.rt.Requests, 1)
		msg := string(f.rt.Requests[0].Input.Message)
		assert.Less(t, strings.Index(msg, "one"), strings.Index(msg, "two"))
		assert.Less(t, strings.Index(msg, "two"), strings.Index(msg, "three"))
		assert.Empty(t, f.store.LeaseOwner(f.thread), "the lease is released")
	})
}

func TestTurnRunner_CancelAndDrain(t *testing.T) {
	t.Run("a cancel is seen within one heartbeat", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newSvc(t)
			f.say(t, "hello")
			f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply")), Block: true})
			r := run.NewRunner(f.svc, f.store, runnerOpts())
			stop := startRunner(t, r)
			defer stop()
			require.True(t, r.Offer(f.kick()))
			synctest.Wait()
			require.Len(t, f.rt.Requests, 1, "the turn is in flight")
			asked := time.Now()
			require.NoError(t, f.store.RequestCancel(f.ctx, f.thread))
			got := f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusCancelled })
			assert.LessOrEqual(t, time.Since(asked), 20*time.Second)
			assert.True(t, got.Usage.Estimated)
		})
	})
	t.Run("a stop past the drain interrupts and releases", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newSvc(t)
			f.say(t, "hello")
			f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply")), Block: true})
			r := run.NewRunner(f.svc, f.store, runnerOpts())
			stop := startRunner(t, r)
			require.True(t, r.Offer(f.kick()))
			synctest.Wait()
			began := time.Now()
			stop()
			assert.GreaterOrEqual(t, time.Since(began), 25*time.Second, "in-flight turns get the drain")
			require.Len(t, f.store.AllRuns(), 1)
			for _, got := range f.store.AllRuns() {
				assert.Equal(t, run.StatusInterrupted, got.Status)
			}
			assert.Empty(t, f.store.LeaseOwner(f.thread), "the lease is released at once")
		})
	})
}

func TestTurnRunner_FullChannelLeavesPending(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	opts := runnerOpts()
	opts.Buffer = 1
	r := run.NewRunner(f.svc, f.store, opts)
	assert.True(t, r.Offer(f.kick()))
	assert.False(t, r.Offer(f.kick()), "a full channel drops the kick")
	pending, err := f.store.PendingUnleased(f.ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{f.thread}, pending, "the message stays pending for the reconciler")
}

type foreverExecutor struct {
	mu    sync.Mutex
	turns int
}

func (e *foreverExecutor) Execute(context.Context, uuid.UUID, string) (*run.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.turns++
	return &run.Run{Status: run.StatusCompleted}, nil
}

type kicks struct {
	mu  sync.Mutex
	got []run.Kick
}

func (k *kicks) Kick(_ context.Context, kick run.Kick) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.got = append(k.got, kick)
	return nil
}

func TestTurnRunner_FairnessReleasesAfterFive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		exec := &foreverExecutor{}
		k := &kicks{}
		r := run.NewRunner(exec, f.store, runnerOpts()).WithKicker(k)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		synctest.Wait()
		stop()
		assert.Equal(t, 5, exec.turns, "at most five turns per lease")
		assert.Empty(t, f.store.LeaseOwner(f.thread))
		assert.Equal(t, []run.Kick{f.kick()}, k.got, "the thread is kicked again for later")
	})
}

func TestKicker_LocalWhenQueueOff(t *testing.T) {
	f := newSvc(t)
	opts := runnerOpts()
	opts.Buffer = 1
	r := run.NewRunner(f.svc, f.store, opts)
	k := run.NewKicker(queue.Disabled(nil), false, r)
	require.NoError(t, k.Kick(f.ctx, f.kick()))
	assert.False(t, r.Offer(f.kick()), "the kick landed on the runner's own channel")
}

func TestRunTurnHandler_AcksFast(t *testing.T) {
	f := newSvc(t)
	opts := runnerOpts()
	opts.Buffer = 1
	r := run.NewRunner(f.svc, f.store, opts)
	h := r.Handler()
	assert.Equal(t, run.TurnJob, h.Job)
	data := []byte(`{"orgId":"` + f.org.String() + `","threadId":"` + f.thread.String() + `"}`)
	require.NoError(t, h.Handle(f.ctx, queue.Message{Data: data}))
	require.NoError(t, h.Handle(f.ctx, queue.Message{Data: data}), "a full runner still acks: the reconciler covers it")
	assert.Error(t, h.Handle(f.ctx, queue.Message{Data: []byte(`{`)}), "a bad payload is refused")
	assert.False(t, r.Offer(f.kick()), "the first kick is on the channel")
}

type settleFails struct{}

func (settleFails) Execute(context.Context, uuid.UUID, string) (*run.Run, error) {
	return nil, &run.SettleError{Cause: assertErr{}}
}

func TestTurnRunner_SettleFailureKeepsTheLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		r := run.NewRunner(settleFails{}, f.store, runnerOpts())
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		synctest.Wait()
		stop()
		assert.NotEmpty(t, f.store.LeaseOwner(f.thread), "the lease is left to expire, so the sweeper reclaims the run")
	})
}

type countingExec struct {
	mu    sync.Mutex
	calls int
	out   *run.Run
}

func (e *countingExec) Execute(context.Context, uuid.UUID, string) (*run.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return e.out, nil
}

func TestTurnRunner_InterruptedTurnReleasesWithoutRekick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		exec := &countingExec{out: &run.Run{Status: run.StatusInterrupted}}
		k := &kicks{}
		r := run.NewRunner(exec, f.store, runnerOpts()).WithKicker(k)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		synctest.Wait()
		stop()
		assert.Equal(t, 1, exec.calls, "a busy or unreachable runtime is not retried hot in the same lease")
		assert.Empty(t, k.got, "reconcile retries it, not a re-kick")
		assert.Empty(t, f.store.LeaseOwner(f.thread))
	})
}

func TestTurnRunner_BusyRuntimeRunsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.say(t, "hello")
		f.rt.Replies = []fakes.RuntimeReply{{Err: &runtime.BusyError{}}, {Err: &runtime.BusyError{}}}
		k := &kicks{}
		r := run.NewRunner(f.svc, f.store, runnerOpts()).WithKicker(k)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		synctest.Wait()
		stop()
		assert.Equal(t, 1, f.rt.Calls(), "a 503 gives exactly one runtime call per kick")
		assert.Empty(t, k.got)
	})
}

type flakyLeases struct {
	*fakes.RunStore
	mu    sync.Mutex
	beats int
}

func (l *flakyLeases) Heartbeat(context.Context, uuid.UUID, string, time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.beats++
	return false, assertErr{}
}

func TestTurnRunner_HeartbeatErrorsCancelBeforeTheLeaseExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.say(t, "hello")
		f.rt.Replies = []fakes.RuntimeReply{{Stream: cut(stream(t, "plain_reply")), Block: true}}
		leases := &flakyLeases{RunStore: f.store}
		r := run.NewRunner(f.svc, leases, runnerOpts())
		stop := startRunner(t, r)
		defer stop()
		require.True(t, r.Offer(f.kick()))
		began := time.Now()
		got := f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusInterrupted })
		assert.Less(t, time.Since(began), time.Minute, "the turn stops before its lease could expire unseen")
		assert.NotNil(t, got)
	})
}
