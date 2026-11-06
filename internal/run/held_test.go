package run_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func drainSink(s *run.Sink) []runtime.EventType {
	var out []runtime.EventType
	for e := range s.Events() {
		out = append(out, e.EventType())
	}
	return out
}

func (f *svcFixture) holdLease(t *testing.T, text string) string {
	t.Helper()
	token := uuid.NewString()
	got, _, err := f.store.ClaimAndAppend(f.ctx, f.thread, token, time.Minute, userMsg(text, ""))
	require.NoError(t, err)
	require.Equal(t, run.Claimed, got)
	return token
}

func TestRunner_ReserveFullAnswersFalse(t *testing.T) {
	f := newSvc(t)
	opts := runnerOpts()
	opts.MaxConcurrent = 1
	r := run.NewRunner(f.svc, f.store, opts)
	slot, ok := r.Reserve()
	require.True(t, ok)
	_, ok = r.Reserve()
	assert.False(t, ok, "a full runner never blocks the handler")
	slot.Release()
	_, ok = r.Reserve()
	assert.True(t, ok, "a released slot is free again")
}

func TestRunner_SlotReleasedWhenHandlerBails(t *testing.T) {
	f := newSvc(t)
	opts := runnerOpts()
	opts.MaxConcurrent = 1
	r := run.NewRunner(f.svc, f.store, opts)
	slot, ok := r.Reserve()
	require.True(t, ok)
	slot.Release()
	slot.Release()
	again, ok := r.Reserve()
	require.True(t, ok, "a double release does not free a slot twice")
	_, ok = r.Reserve()
	assert.False(t, ok)
	again.Release()
}

func TestRunner_RunHeldFeedsSinkAndSettles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
		r := run.NewRunner(f.svc, f.store, runnerOpts())
		stop := startRunner(t, r)
		defer stop()
		synctest.Wait()
		token := f.holdLease(t, "hello")
		slot, ok := r.Reserve()
		require.True(t, ok)
		sink := run.NewSink(run.SinkCapacity)
		require.NoError(t, r.RunHeld(slot, f.kick(), token, sink))
		got := drainSink(sink)
		assert.Equal(t, runtime.EventRunStarted, got[0])
		assert.Equal(t, runtime.EventRunFinished, got[len(got)-1], "RUN_FINISHED is released only once the run settled")
		assert.Contains(t, got, runtime.EventTerminal, "the sink sees the runtime's own stream; the relay filters it")
		synctest.Wait()
		runs := f.store.AllRuns()
		require.Len(t, runs, 1)
		assert.Equal(t, run.StatusCompleted, runs[0].Status)
		assert.Empty(t, f.store.LeaseOwner(f.thread), "the held lease is released like a kicked one")
		assert.False(t, sink.Detached())
	})
}

func TestRunner_SlowSinkDetachesRunSettles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "tool_call")}}
		r := run.NewRunner(f.svc, f.store, runnerOpts())
		stop := startRunner(t, r)
		defer stop()
		synctest.Wait()
		token := f.holdLease(t, "record lunch")
		slot, _ := r.Reserve()
		sink := run.NewSink(1)
		require.NoError(t, r.RunHeld(slot, f.kick(), token, sink))
		synctest.Wait()
		assert.True(t, sink.Detached(), "a client that does not read loses its stream")
		runs := f.store.AllRuns()
		require.Len(t, runs, 1)
		assert.Equal(t, run.StatusCompleted, runs[0].Status, "never the run")
	})
}

type twoTurns struct {
	mu    sync.Mutex
	plain int
	held  int
}

func (e *twoTurns) Execute(context.Context, uuid.UUID, string) (*run.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plain++
	if e.plain == 1 {
		return &run.Run{Status: run.StatusCompleted}, nil
	}
	return nil, nil
}

func (e *twoTurns) ExecuteObserved(_ context.Context, _ uuid.UUID, _ string, obs run.Observer) (*run.Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.held++
	obs(&runtime.RunStarted{Type: runtime.EventRunStarted})
	obs(&runtime.RunFinished{Type: runtime.EventRunFinished})
	return &run.Run{Status: run.StatusCompleted}, nil
}

func TestRunner_SinkClosedAfterItsRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		exec := &twoTurns{}
		r := run.NewRunner(exec, f.store, runnerOpts())
		stop := startRunner(t, r)
		defer stop()
		synctest.Wait()
		token := f.holdLease(t, "hello")
		slot, _ := r.Reserve()
		sink := run.NewSink(run.SinkCapacity)
		require.NoError(t, r.RunHeld(slot, f.kick(), token, sink))
		assert.Equal(t, []runtime.EventType{runtime.EventRunStarted, runtime.EventRunFinished}, drainSink(sink),
			"the sink belongs to the POST's run only")
		synctest.Wait()
		assert.Equal(t, 1, exec.held)
		assert.Equal(t, 2, exec.plain, "the lease goes on with later turns that never reach the sink")
	})
}

func TestRunner_DrainEndsSinkWithInterrupted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newSvc(t)
		f.rt.Replies = []fakes.RuntimeReply{{Stream: cut(stream(t, "plain_reply")), Block: true}}
		r := run.NewRunner(f.svc, f.store, runnerOpts())
		stop := startRunner(t, r)
		synctest.Wait()
		token := f.holdLease(t, "hello")
		slot, _ := r.Reserve()
		sink := run.NewSink(run.SinkCapacity)
		require.NoError(t, r.RunHeld(slot, f.kick(), token, sink))
		synctest.Wait()
		stop()
		var last runtime.Event
		for e := range sink.Events() {
			last = e
		}
		runErr, ok := last.(*runtime.RunError)
		require.True(t, ok, "the stream ends with RUN_ERROR, got %T", last)
		assert.Equal(t, "interrupted", runErr.Code)
		assert.Equal(t, run.StatusInterrupted, f.store.AllRuns()[0].Status)
	})
}

func TestRunner_RunHeldRefusedWhenNotRunning(t *testing.T) {
	f := newSvc(t)
	r := run.NewRunner(f.svc, f.store, runnerOpts())
	slot, ok := r.Reserve()
	require.True(t, ok)
	assert.Error(t, r.RunHeld(slot, f.kick(), "tok", run.NewSink(1)), "a runner that is not running takes no held lease")
	_, ok = r.Reserve()
	assert.True(t, ok, "the slot came back")
}

func TestExecuteObserved_SynthesizesTheEndOnACut(t *testing.T) {
	for name, c := range map[string]struct {
		cause error
		code  string
	}{
		"cancel":   {run.ErrCancelRequested, "cancelled"},
		"shutdown": {context.Canceled, "interrupted"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSvc(t)
			f.say(t, "hello")
			f.rt.Replies = []fakes.RuntimeReply{{Stream: cut(stream(t, "plain_reply")), Block: true}}
			token := f.claim(t)
			ctx, cancel := context.WithCancelCause(f.ctx)
			cancel(c.cause)
			var events []runtime.Event
			_, err := f.svc.ExecuteObserved(ctx, f.thread, token, func(e runtime.Event) { events = append(events, e) })
			require.NoError(t, err)
			runErr, ok := events[len(events)-1].(*runtime.RunError)
			require.True(t, ok)
			assert.Equal(t, c.code, runErr.Code)
			assert.NotEmpty(t, runErr.Message)
		})
	}
}

func TestExecuteObserved_FailedRunEndsWithItsCode(t *testing.T) {
	f := newSvc(t)
	f.version.Legacy = true
	f.say(t, "hello")
	token := f.claim(t)
	var events []runtime.Event
	_, err := f.svc.ExecuteObserved(f.ctx, f.thread, token, func(e runtime.Event) { events = append(events, e) })
	require.NoError(t, err)
	require.Len(t, events, 1)
	runErr, ok := events[0].(*runtime.RunError)
	require.True(t, ok)
	assert.Equal(t, "AGT006", runErr.Code)
}

func TestExecuteObserved_HeldRunSkipsTheRateCap(t *testing.T) {
	f := newSvc(t)
	f.svc.WithRateCap(run.RateCap{Limiter: refuseAll{}, PerMinute: 1})
	f.say(t, "hello")
	f.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	token := f.claim(t)
	got, err := f.svc.ExecuteObserved(f.ctx, f.thread, token, func(runtime.Event) {})
	require.NoError(t, err, "the POST already paid the rate charge")
	assert.Equal(t, run.StatusCompleted, got.Status)
}

type refuseAll struct{}

func (refuseAll) Allow(context.Context, string, ratelimit.Rule) (ratelimit.Result, error) {
	return ratelimit.Result{Allowed: false, RetryAfter: time.Minute}, nil
}

func (refuseAll) Reset(context.Context, string) error { return nil }
