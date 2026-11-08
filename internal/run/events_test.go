package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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

	require.NoError(t, f.svc.Cancel(f.ctx, r.ID))
	runs := f.store.AllRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, run.StatusCancelled, runs[0].Status)
	require.Len(t, pub.Events(), 1)
	assert.Equal(t, events.RunFailed, pub.Events()[0].Type, "a cancel always tells the subscribers")
	assert.Empty(t, f.store.LeaseOwner(f.thread), "the cancel's own lease is released")
	require.NoError(t, f.svc.Cancel(f.ctx, r.ID), "a settled run is left as it is")
	assert.Len(t, pub.Events(), 1, "and emits nothing again")
}

func TestCancel_LiveTurnIsAskedToStop(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	token := f.claim(t)
	r := run.New(f.org, f.project, f.thread, f.version.ID, time.Now())
	_, err := f.store.StartRun(f.ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, f.svc.Cancel(f.ctx, r.ID))
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

func TestCancel_UnknownRunIsNotFound(t *testing.T) {
	f := newSvc(t)
	assert.True(t, run.IsNotFoundError(f.svc.Cancel(f.ctx, uuid.New())))
}

func TestCancel_SettledRunIsNoOp(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.Equal(t, run.StatusCompleted, r.Status)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)

	require.NoError(t, f.svc.Cancel(f.ctx, r.ID))
	runs := f.store.AllRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, run.StatusCompleted, runs[0].Status, "a completed run stays completed")
	assert.Empty(t, pub.Events(), "nothing is emitted")
	th, err := f.store.Thread(f.ctx, f.thread)
	require.NoError(t, err)
	assert.False(t, th.CancelRequested, "no cancel is left behind for the next turn")
	assert.Empty(t, f.store.LeaseOwner(f.thread))

	f.claim(t)
	require.NoError(t, f.svc.Cancel(f.ctx, r.ID))
	th, err = f.store.Thread(f.ctx, f.thread)
	require.NoError(t, err)
	assert.False(t, th.CancelRequested, "cancelling an old run never stops the turn that holds the thread now")
}

type staleRunRead struct {
	*fakes.RunStore
	stale run.Run
}

func (s staleRunRead) RunByID(ctx context.Context, id uuid.UUID) (*run.Run, error) {
	if id == s.stale.ID {
		cp := s.stale
		return &cp, nil
	}
	return s.RunStore.RunByID(ctx, id)
}

func TestCancel_ClaimedPathOnlySettlesTheRunAskedFor(t *testing.T) {
	f := newSvc(t)
	f.say(t, "hello")
	a := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.Equal(t, run.StatusCompleted, a.Status)
	f.say(t, "again")
	b := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply"))})
	require.Equal(t, run.StatusInterrupted, b.Status, "B is the thread's open run now")
	stale := *a
	stale.Status = run.StatusRunning
	pub := &fakes.Publisher{}
	svc := run.NewService(staleRunRead{RunStore: f.store, stale: stale}, fakes.Transactor{}, nil).
		WithAgents(f.agents).WithPublisher(pub)

	require.NoError(t, svc.Cancel(f.ctx, a.ID), "A was read as live just before it ended")
	for _, r := range f.store.AllRuns() {
		if r.ID == b.ID {
			assert.Equal(t, run.StatusInterrupted, r.Status, "B is never settled by a cancel of A")
		}
	}
	assert.Empty(t, pub.Events())
}

func TestTryRun_PinsTheDraftAndEmitsNoWebhook(t *testing.T) {
	f := newSvc(t)
	pub := &fakes.Publisher{}
	f.svc.WithPublisher(pub)
	f.store.Kinds = map[uuid.UUID]string{f.thread: "try"}
	draft := *f.version
	draft.ID, draft.Version, draft.Draft, draft.Instructions = uuid.New(), 0, true, "The unpublished draft."
	f.agents.Drafts = map[uuid.UUID]*run.AgentVersion{f.agentID: &draft}
	f.agents.Versions[draft.ID] = &draft
	f.say(t, "hello")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, draft.ID, r.AgentVersionID, "a try run is pinned to the draft, not the published version")
	require.NotEmpty(t, f.rt.Requests)
	assert.Equal(t, "The unpublished draft.", f.rt.Requests[0].Agent.Instructions)
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Empty(t, pub.Events(), "a try run sends no webhook")
	assert.Positive(t, r.Usage.InputTokens, "its usage is recorded like any run")
}

func TestTryRun_WaitsWithoutAPinnableDraft(t *testing.T) {
	f := newSvc(t)
	f.store.Kinds = map[uuid.UUID]string{f.thread: "try"}
	f.agents.DraftErr = assertErr{}
	f.say(t, "hello")
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	require.Error(t, err)
	assert.Empty(t, f.store.Runs, "no run starts on a draft that can't be pinned")
}

func TestTryRun_KeepsItsDraftRowWhenTheDraftIsEdited(t *testing.T) {
	f := newSvc(t)
	f.store.Kinds = map[uuid.UUID]string{f.thread: "try"}
	draftA, draftB := *f.version, *f.version
	draftA.ID, draftA.Version, draftA.Draft, draftA.Instructions = uuid.New(), 0, true, "Draft A."
	draftB.ID, draftB.Version, draftB.Draft, draftB.Instructions = uuid.New(), 0, true, "Draft B."
	f.agents.Versions[draftA.ID], f.agents.Versions[draftB.ID] = &draftA, &draftB
	f.agents.Drafts = map[uuid.UUID]*run.AgentVersion{f.agentID: &draftA}

	f.say(t, "record lunch")
	paused := f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	require.Equal(t, run.StatusInterrupted, paused.Status)
	assert.Equal(t, draftA.ID, paused.AgentVersionID)

	f.agents.Drafts[f.agentID] = &draftB
	resumed := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Equal(t, paused.ID, resumed.ID, "the same run resumes")
	assert.Equal(t, draftA.ID, resumed.AgentVersionID, "a paused try run keeps the draft row it started on")
	assert.Equal(t, "Draft A.", f.rt.LastRequest().Agent.Instructions, "and resumes on that row's config, not the edited draft")
	require.Equal(t, run.StatusCompleted, resumed.Status)

	f.say(t, "and dinner")
	next := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.NotEqual(t, paused.ID, next.ID)
	assert.Equal(t, draftB.ID, next.AgentVersionID, "a new try run pins the edited draft")
	assert.Equal(t, "Draft B.", f.rt.LastRequest().Agent.Instructions)
}
