package approval_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
)

type refs struct {
	*fakes.PublicIDs
	threads, runs map[uuid.UUID]string
}

func (r refs) ThreadPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return r.threads[id], nil
}
func (r refs) RunPublicID(_ context.Context, id uuid.UUID) (string, error) { return r.runs[id], nil }

type kicks struct {
	mu      sync.Mutex
	threads []uuid.UUID
}

func (k *kicks) Kick(_ context.Context, _, threadID uuid.UUID) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.threads = append(k.threads, threadID)
	return nil
}

type world struct {
	svc                                     *approval.Service
	store                                   *fakes.ApprovalStore
	pub                                     *fakes.Publisher
	kicks                                   *kicks
	now                                     time.Time
	org, project, agent, run, thread        uuid.UUID
	projectPub, agentPub, threadPub, runPub string
	ctx                                     context.Context
	ids                                     *fakes.PublicIDs
	refs                                    refs
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		store: &fakes.ApprovalStore{}, pub: &fakes.Publisher{}, kicks: &kicks{}, now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC),
		org: uuid.New(), project: uuid.New(), agent: uuid.New(), run: uuid.New(), thread: uuid.New(),
		threadPub: "ThQ2wE4rT6yU8jP9pA3sD", runPub: "RnZ3xC5vB7nM9qW2eR2tY",
	}
	ids := fakes.NewPublicIDs()
	ids.Org(w.org)
	w.projectPub = ids.Project(w.project, w.org)
	w.agentPub = ids.Resource(w.agent, w.org)
	r := refs{PublicIDs: ids, threads: map[uuid.UUID]string{w.thread: w.threadPub}, runs: map[uuid.UUID]string{w.run: w.runPub}}
	w.ids, w.refs = ids, r
	w.svc = approval.NewService(w.store, fakes.Transactor{}, r, apperror.NewReporter(nil, false).Unexpected).
		WithPublisher(w.pub).WithKicker(w.kicks).WithClock(func() time.Time { return w.now })
	w.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: w.org, ProjectID: w.project})
	return w
}

func expenseCall() approval.Call {
	return approval.Call{
		ToolCallID: "call_1", InterruptID: "intr_1", Server: "yasaku", Tool: "record_expense",
		Args: json.RawMessage(`{"amount":25000,"note":"*kopi* _susu_","confirm":true}`), ArgsHash: "abc", Keys: []string{"amount", "note", "confirm"},
	}
}

func (w *world) create(t *testing.T, kind string, speakers ...uuid.NullUUID) *approval.Approval {
	t.Helper()
	a, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: w.run, ThreadID: w.thread, ThreadKind: kind,
		Calls: []approval.Call{expenseCall()}, Speakers: speakers,
	})
	require.NoError(t, err)
	return a
}

func TestApproval_ExpiryFromThreadKind(t *testing.T) {
	for kind, tc := range map[string]struct {
		wait    time.Duration
		channel approval.Channel
	}{
		"group": {10 * time.Minute, approval.ChannelWhatsApp},
		"dm":    {24 * time.Hour, approval.ChannelWhatsApp},
		"api":   {24 * time.Hour, approval.ChannelAPI},
		"try":   {24 * time.Hour, approval.ChannelConsole},
	} {
		w := newWorld(t)
		a := w.create(t, kind)
		assert.Equal(t, w.now.Add(tc.wait), a.ExpiresAt, kind)
		assert.Equal(t, tc.channel, a.Channel, kind)
		assert.Equal(t, approval.StatusPending, a.Status, kind)
		assert.Len(t, a.PublicID, 21, kind)
	}
}

func TestApproval_GroupRequesterIsLastSpeaker(t *testing.T) {
	sari, budi := uuid.NullUUID{UUID: uuid.New(), Valid: true}, uuid.NullUUID{UUID: uuid.New(), Valid: true}
	w := newWorld(t)
	assert.Equal(t, budi, w.create(t, "group", sari, budi).Requester, "the speaker of the turn's last taken message decides")
	w = newWorld(t)
	assert.False(t, w.create(t, "api").Requester.Valid, "an API turn has no end-user speaker")
}

func TestApproval_DecideIsSingleUse(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "api")
	got, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true, By: approval.Decider{Kind: approval.DeciderAPIKey, ID: "key"}})
	require.NoError(t, err)
	assert.Equal(t, approval.StatusApproved, got.Status)
	assert.Equal(t, approval.CallApproved, got.Calls[0].State)
	_, err = w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: false})
	var decided *approval.AlreadyDecidedError
	require.ErrorAs(t, err, &decided, "a second decision changes nothing")
	assert.Equal(t, "approved", decided.Status)

	w = newWorld(t)
	a = w.create(t, "api")
	var wins, losses int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: i%2 == 0})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if approval.IsAlreadyDecidedError(err) {
				losses++
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 1, wins, "one of eight concurrent decisions wins")
	assert.Equal(t, 7, losses)
}

func TestApproval_DecideAfterExpiryIsAPR003(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "group")
	w.now = w.now.Add(10 * time.Minute)
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true})
	var decided *approval.AlreadyDecidedError
	require.ErrorAs(t, err, &decided)
	assert.Equal(t, "expired", decided.Status, "the console tells expired apart from decided")
	assert.Equal(t, apperror.CodeApprovalAlreadyDecided, decided.ToAppError().Code())

	_, err = w.svc.Decide(w.ctx, uuid.New(), approval.Answer{Approved: true})
	assert.True(t, approval.IsNotFoundError(err), "an unknown approval")
}

func TestApproval_EventsInTheUnitOfWork(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "dm")
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: false, Text: "no, it was 20000"})
	require.NoError(t, err)
	got := w.pub.Events()
	require.Len(t, got, 2)
	want := events.ApprovalV1{
		ID: a.PublicID, ProjectID: w.projectPub, AgentID: w.agentPub, ThreadID: w.threadPub, RunID: w.runPub,
		Tools: []string{"record_expense"}, Channel: "whatsapp",
	}
	want.Status = "pending"
	assert.Equal(t, events.ApprovalRequested, got[0].Type)
	assert.Equal(t, events.ApprovalRequestedV1(want), got[0].Data)
	want.Status = "rejected"
	assert.Equal(t, events.ApprovalDecided, got[1].Type)
	assert.Equal(t, events.ApprovalDecidedV1(want), got[1].Data)
	for _, e := range got {
		assert.True(t, e.InTx, "%s is enqueued in the write's unit of work", e.Type)
		assert.Equal(t, tenant.Context{OrgID: w.org, ProjectID: w.project}, e.Tenant)
	}

	w = newWorld(t)
	w.pub.Err = errors.New("outbox down")
	_, err = w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: w.run, ThreadID: w.thread, ThreadKind: "api", Calls: []approval.Call{expenseCall()},
	})
	assert.Error(t, err, "a failed enqueue fails the create")
}

func TestApproval_TryThreadsEmitNoEvent(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "try")
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true, By: approval.Decider{Kind: approval.DeciderUser, ID: "usr"}})
	require.NoError(t, err)
	assert.Empty(t, w.pub.Events(), "a try run sends no webhook, so neither approval event")
}

func TestApproval_KickOnlyWhenAsked(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "dm")
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true, Kick: false})
	require.NoError(t, err)
	assert.Empty(t, w.kicks.threads, "the API path holds the lease and streams the resume itself")

	w = newWorld(t)
	a = w.create(t, "dm")
	_, err = w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true, Kick: true})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{w.thread}, w.kicks.threads)
}

func TestApproval_DecisionTextIsCapped(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "dm")
	got, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Text: strings.Repeat("é", 4001)})
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("é", 4000), got.DecisionText)
}

func TestApproval_CallStates(t *testing.T) {
	w := newWorld(t)
	a := w.create(t, "api")
	assert.Error(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}), "an undecided call never executes")
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true})
	require.NoError(t, err)
	require.NoError(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}))
	require.NoError(t, w.svc.MarkUndelivered(w.ctx, a.ID, []string{"call_1"}), "a request that never left goes back to approved")
	assert.Error(t, w.svc.MarkUndelivered(w.ctx, a.ID, []string{"call_1"}), "only an executing call goes back")
	require.NoError(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}))
	assert.Error(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}), "executing is entered once")
	require.NoError(t, w.svc.MarkExecuted(w.ctx, a.ID, "call_1"))
	assert.Error(t, w.svc.MarkUnknown(w.ctx, a.ID, "call_1"), "an executed call is never unknown")
	got, err := w.svc.Get(w.ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.CallExecuted, got.Calls[0].State)
}

func TestApproval_PendingAndList(t *testing.T) {
	w := newWorld(t)
	_, err := w.svc.PendingForThread(w.ctx, w.thread)
	assert.True(t, approval.IsNotFoundError(err))
	a := w.create(t, "api")
	got, err := w.svc.PendingForThread(w.ctx, w.thread)
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.ID)
	list, err := w.svc.List(w.ctx, approval.Filter{ProjectID: w.project, Status: approval.StatusPending}, approval.Cursor{}, 10)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	_, err = w.svc.Create(w.ctx, approval.CreateInput{ThreadID: w.thread, ThreadKind: "api"})
	assert.Error(t, err, "an approval needs at least one call")
}
