package run_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestSettle_EmitsEventInSameTx(t *testing.T) {
	for _, c := range []struct {
		name   string
		golden string
		cut    bool
		want   events.Type
		status string
	}{
		{"completed", "plain_reply", false, events.RunCompleted, "completed"},
		{"limit reached", "limit_reached", false, events.RunCompleted, "limit_reached"},
		{"runtime failure", "model_error", false, events.RunFailed, "failed"},
		{"deadline", "cancel", false, events.RunFailed, "failed"},
		{"interrupted", "plain_reply", true, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSvc(t)
			pub := &fakes.Publisher{}
			f.svc.WithPublisher(pub)
			f.say(t, "hello")
			raw := stream(t, c.golden)
			if c.cut {
				raw = cut(raw)
			}
			r := f.execute(t, fakes.RuntimeReply{Stream: raw})
			got := pub.Events()
			if c.want == "" {
				assert.Empty(t, got, "an interrupted run emits nothing")
				return
			}
			require.Len(t, got, 1)
			e := got[0]
			assert.Equal(t, c.want, e.Type)
			assert.True(t, e.InTx, "the event is enqueued inside the settle transaction")
			assert.Equal(t, f.org, e.Tenant.OrgID)
			assert.Equal(t, f.project, e.Tenant.ProjectID)
			require.NoError(t, events.CheckPayload(e.Type, e.Data))
			var p events.RunSettledV1
			switch d := e.Data.(type) {
			case events.RunCompletedV1:
				p = events.RunSettledV1(d)
			case events.RunFailedV1:
				p = events.RunSettledV1(d)
			}
			assert.Equal(t, r.PublicID, p.RunID)
			assert.Equal(t, "thread-pub", p.ThreadID, "public ids, never a uuid or a session id")
			assert.Equal(t, "agent-pub", p.AgentID)
			assert.Equal(t, c.status, p.Status)
			assert.Equal(t, r.Usage.InputTokens, p.Usage.InputTokens)
			micros, priced := run.Cost(r.Usage, r.Prices)
			require.True(t, priced)
			require.NotNil(t, p.CostMicros)
			assert.Equal(t, micros, *p.CostMicros)
		})
	}
}

func TestSettle_RefusalAndCancelEmitFailed(t *testing.T) {
	f := newSvc(t)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)
	f.version.Legacy = true
	f.say(t, "hello")
	f.execute(t)
	require.Len(t, pub.Events(), 1)
	assert.Equal(t, events.RunFailed, pub.Events()[0].Type)
}

func TestSettle_PublisherFailureFailsTheSettle(t *testing.T) {
	f := newSvc(t)
	f.svc.WithPublisher(&fakes.Publisher{Err: assertErr{}})
	f.say(t, "hello")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	assert.Error(t, err)
}

type assertErr struct{}

func (assertErr) Error() string { return "publisher down" }

func TestCancel_IdleThreadSettlesAndEmits(t *testing.T) {
	f := newSvc(t)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
	require.Equal(t, run.StatusInterrupted, r.Status)
	require.Empty(t, pub.Events(), "an interrupted run emits nothing")

	require.NoError(t, f.svc.Cancel(f.ctx, f.thread))
	runs := f.store.AllRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, run.StatusCancelled, runs[0].Status)
	require.Len(t, pub.Events(), 1)
	assert.Equal(t, events.RunFailed, pub.Events()[0].Type, "a cancel always tells the subscribers")
	assert.Empty(t, f.store.LeaseOwner(f.thread), "the cancel's own lease is released")
	require.NoError(t, f.svc.Cancel(f.ctx, f.thread), "nothing left to cancel is fine")
}

func TestCancel_LiveTurnIsAskedToStop(t *testing.T) {
	f := newSvc(t)
	token := f.claim(t)
	require.NoError(t, f.svc.Cancel(f.ctx, f.thread))
	stop, err := f.store.Heartbeat(f.ctx, f.thread, token, time.Minute)
	require.NoError(t, err)
	assert.True(t, stop, "the turn holding the lease sees the request at its next heartbeat")
}

func TestEventFor_EveryStatus(t *testing.T) {
	want := map[run.Status]events.Type{
		run.StatusCompleted: events.RunCompleted, run.StatusLimitReached: events.RunCompleted,
		run.StatusFailed: events.RunFailed, run.StatusCancelled: events.RunFailed, run.StatusExpired: events.RunFailed,
		run.StatusRunning: "", run.StatusInterrupted: "", run.StatusAwaitingApproval: "",
	}
	require.Len(t, want, len(run.Statuses()))
	for _, s := range run.Statuses() {
		got := run.EventFor(s)
		assert.Equal(t, want[s], got, "%s", s)
		assert.Equal(t, s.Terminal(), got != "", "%s: every terminal status emits, and only those", s)
	}
}
