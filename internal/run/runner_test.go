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
			require.Len(t, f.store.AllRuns(), 1)
			require.NoError(t, f.store.RequestCancel(f.ctx, f.thread, f.store.AllRuns()[0].ID))
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

type quietWindows struct {
	mu       sync.Mutex
	w, wmax  time.Duration
	arrivals []time.Time
}

func (q *quietWindows) arrive() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.arrivals = append(q.arrivals, time.Now())
}

func (q *quietWindows) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.arrivals = nil
}

func (q *quietWindows) QuietWait(_ context.Context, _ uuid.UUID, now time.Time) (time.Duration, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.arrivals) == 0 {
		return 0, nil
	}
	return run.QuietWait(now, q.arrivals[0], q.arrivals[len(q.arrivals)-1], q.w, q.wmax), nil
}

type drainExec struct {
	countingExec
	q *quietWindows
}

func (e *drainExec) Execute(ctx context.Context, id uuid.UUID, token string) (*run.Run, error) {
	e.q.clear()
	return e.countingExec.Execute(ctx, id, token)
}

func (e *drainExec) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func TestQuietWait(t *testing.T) {
	t0 := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, 3*time.Second, run.QuietWait(t0, t0, t0, 3*time.Second, 10*time.Second))
	assert.Equal(t, time.Second, run.QuietWait(t0.Add(2*time.Second), t0, t0, 3*time.Second, 10*time.Second))
	assert.Equal(t, time.Second, run.QuietWait(t0.Add(9*time.Second), t0, t0.Add(8*time.Second), 3*time.Second, 10*time.Second), "the cap wins")
	assert.Zero(t, run.QuietWait(t0.Add(10*time.Second), t0, t0.Add(9*time.Second), 3*time.Second, 10*time.Second))
	assert.Zero(t, run.QuietWait(t0, t0, t0, 0, 0), "an API thread never waits")
}

func TestRunner_QuietWindow(t *testing.T) {
	t.Run("no turn before W", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newSvc(t)
			q := &quietWindows{w: 3 * time.Second, wmax: 10 * time.Second}
			exec := &drainExec{q: q}
			r := run.NewRunner(exec, f.store, runnerOpts()).WithWindows(q)
			stop := startRunner(t, r)
			defer stop()
			q.arrive()
			require.True(t, r.Offer(f.kick()))
			time.Sleep(2900 * time.Millisecond)
			synctest.Wait()
			assert.Zero(t, exec.count(), "the window is still open")
			assert.Empty(t, f.store.LeaseOwner(f.thread), "the lease is released while waiting")
			time.Sleep(200 * time.Millisecond)
			synctest.Wait()
			assert.Equal(t, 1, exec.count(), "one turn once the thread is quiet")
		})
	})
	t.Run("one turn by Wmax", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newSvc(t)
			q := &quietWindows{w: 3 * time.Second, wmax: 10 * time.Second}
			exec := &drainExec{q: q}
			r := run.NewRunner(exec, f.store, runnerOpts()).WithWindows(q)
			stop := startRunner(t, r)
			defer stop()
			for range 5 {
				q.arrive()
				r.Offer(f.kick())
				time.Sleep(1900 * time.Millisecond)
				synctest.Wait()
				assert.Zero(t, exec.count(), "a steady stream keeps the window open")
				time.Sleep(100 * time.Millisecond)
			}
			synctest.Wait()
			synctest.Wait()
			assert.Equal(t, 1, exec.count(), "the oldest message never waits past Wmax")
		})
	})
	t.Run("timers stop with Run", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newSvc(t)
			q := &quietWindows{w: 3 * time.Second, wmax: 10 * time.Second}
			exec := &drainExec{q: q}
			r := run.NewRunner(exec, f.store, runnerOpts()).WithWindows(q)
			stop := startRunner(t, r)
			q.arrive()
			require.True(t, r.Offer(f.kick()))
			time.Sleep(time.Second)
			synctest.Wait()
			stop()
			time.Sleep(time.Second)
			again := startRunner(t, r)
			time.Sleep(5 * time.Second)
			synctest.Wait()
			again()
			assert.Zero(t, exec.count(), "a timer from the stopped Run never kicks")
		})
	})
}

type replies struct {
	mu   sync.Mutex
	runs []uuid.UUID
	err  error
}

func (s *replies) Send(_ context.Context, runID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, runID)
	return s.err
}

func (s *replies) sent() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uuid.UUID(nil), s.runs...)
}

func TestTurnRunner_SendsTheDueReplyAfterSettle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		_, conn := f.channel(t, "dm", true)
		f.withTexts(conn)
		f.say(t, "halo")
		f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		sender := &replies{err: assert.AnError}
		r := run.NewRunner(f.svc, f.store, runnerOpts()).WithReplies(sender)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		done := f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusCompleted })
		synctest.Wait()
		stop()
		assert.Equal(t, []uuid.UUID{done.ID}, sender.sent(), "the runner sends right after the settle")
		assert.Empty(t, f.store.LeaseOwner(f.thread), "a failed send leaves the reply to the job")
	})
}

func TestTurnRunner_APIThreadSendsNoReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.say(t, "halo")
		f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		sender := &replies{}
		r := run.NewRunner(f.svc, f.store, runnerOpts()).WithReplies(sender)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusCompleted })
		synctest.Wait()
		stop()
		assert.Empty(t, sender.sent())
	})
}

func TestTurnRunner_SendsTheApprovalPrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		_, conn := f.channel(t, "dm", true)
		f.withTexts(conn)
		f.withApprovals()
		f.say(t, "record lunch 25000")
		f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
		sender := &replies{}
		r := run.NewRunner(f.svc, f.store, runnerOpts()).WithReplies(sender)
		stop := startRunner(t, r)
		require.True(t, r.Offer(f.kick()))
		done := f.waitRun(t, func(r *run.Run) bool { return r.Status == run.StatusAwaitingApproval })
		synctest.Wait()
		stop()
		assert.Equal(t, run.ReplyKindPrompt, done.ReplyKind)
		assert.Equal(t, []uuid.UUID{done.ID}, sender.sent(), "the prompt goes out right after the awaiting settle")
	})
}
