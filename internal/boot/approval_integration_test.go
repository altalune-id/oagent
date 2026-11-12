//go:build integration

package boot

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
)

func TestRunApprovals_RaiseThenLatest(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-r", "Org R", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, _ := seedThread(in, t, f, prj.ID, ag.ID)
	a := runApprovals{svcs: f.svcs}

	got, err := a.Latest(in, r.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "a run with no approval has none")

	call := run.ApprovalCall{
		ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "record_expense",
		Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"},
	}
	require.NoError(t, a.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind,
		Calls: []run.ApprovalCall{call}, SnapshotHash: []byte{7, 7},
	}))
	got, err = a.Latest(in, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, []byte{7, 7}, got.RaisedSnapshotHash)
	assert.False(t, got.ExpiresAt.IsZero())
	require.Len(t, got.Calls, 1)
	assert.Equal(t, call.InterruptID, got.Calls[0].InterruptID)
	assert.Equal(t, call.ArgsHash, got.Calls[0].ArgsHash)
	assert.Equal(t, call.Keys, got.Calls[0].Keys)
	assert.JSONEq(t, string(call.Args), string(got.Calls[0].Args))
}

func TestApprovalPrompts_PendingThenSent(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-p", "Org P", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, _ := seedThread(in, t, f, prj.ID, ag.ID)
	p := approvalPrompts{svcs: f.svcs}

	got, err := p.Prompt(in, r.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "no approval, no prompt")

	require.NoError(t, runApprovals{svcs: f.svcs}.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind, SnapshotHash: []byte{1},
		Calls: []run.ApprovalCall{{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "record_expense",
			Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}}},
	}))
	got, err = p.Prompt(in, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotEmpty(t, got.PublicID)
	assert.NotEmpty(t, got.Preview)

	require.NoError(t, p.PromptSent(in, got.ApprovalID, "", 3))
	a, err := f.svcs.Approvals.Get(in, got.ApprovalID)
	require.NoError(t, err)
	assert.Empty(t, a.PromptExternalID)
	var stored *string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT prompt_external_id FROM approvals WHERE id = $1`, got.ApprovalID).Scan(&stored))
	assert.Nil(t, stored, "an empty provider id is stored as NULL, so a retry can still fill it")
	require.NotNil(t, a.PromptAfterSeq)
	assert.Equal(t, int64(3), *a.PromptAfterSeq)

	require.NoError(t, p.PromptSent(in, got.ApprovalID, "wamid-prompt", 5))
	require.NoError(t, p.PromptSent(in, got.ApprovalID, "wamid-later", 7))
	a, err = f.svcs.Approvals.Get(in, got.ApprovalID)
	require.NoError(t, err)
	assert.Equal(t, "wamid-prompt", a.PromptExternalID, "a retry can fill a missing id; the first id is kept")
	assert.Equal(t, int64(3), *a.PromptAfterSeq, "the first watermark is kept")
}

// SECURITY: the chat decision and the consumed answer share one unit of work, so a lost lease leaves the approval pending.
func TestRunApprovals_LostLeaseRollsTheChatDecisionBack(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-l", "Org L", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, who := seedThread(in, t, f, prj.ID, ag.ID)
	a := runApprovals{svcs: f.svcs}
	require.NoError(t, a.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind, SnapshotHash: []byte{1},
		Speakers: []uuid.NullUUID{{UUID: who.ID, Valid: true}},
		Calls: []run.ApprovalCall{{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "record_expense",
			Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}}},
	}))
	pending, err := a.Latest(in, r.ID)
	require.NoError(t, err)

	err = db.NewTransactor(f.pool).RunInTx(in, func(ctx context.Context) error {
		ok, err := a.DecideFromChat(ctx, run.ChatAnswer{ApprovalID: pending.ID, By: who.ID, Approved: true})
		require.NoError(t, err)
		require.True(t, ok)
		return f.svcs.RunStore.AnswerMessage(ctx, "not-the-lease", th.ID, uuid.New(), r.ID)
	})
	var lost *run.LeaseLostError
	require.ErrorAs(t, err, &lost)
	got, err := a.Latest(in, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", got.Status, "the decision rolled back with the lost lease")
}

func TestRunApprovals_CancelRejectsAsTheCanceller(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-c", "Org C", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, _ := seedThread(in, t, f, prj.ID, ag.ID)
	a := runApprovals{svcs: f.svcs}
	require.NoError(t, a.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind, SnapshotHash: []byte{1},
		Calls: []run.ApprovalCall{{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "record_expense",
			Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}}},
	}))
	pending, err := a.Latest(in, r.ID)
	require.NoError(t, err)

	canceller := uuid.New()
	require.NoError(t, a.RejectOnCancel(session.PrincipalInto(in, session.Principal{UserID: canceller, Source: session.SourceLocal}), pending.ID, false))
	got, err := f.svcs.Approvals.Get(in, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusRejected, got.Status)
	assert.Equal(t, approval.DeciderUser, got.DecidedByKind)
	assert.Equal(t, canceller.String(), got.DecidedByID, "decided by the canceller")
	require.NoError(t, a.RejectOnCancel(in, pending.ID, false), "a decided approval is left as it is")

	ag2, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot-2", Name: "Bot 2"})
	require.NoError(t, err)
	th2, again, _ := seedThread(in, t, f, prj.ID, ag2.ID)
	require.NoError(t, a.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag2.ID, RunID: again.ID, ThreadID: th2.ID, ThreadKind: th2.Kind, SnapshotHash: []byte{1},
		Calls: []run.ApprovalCall{{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "record_expense",
			Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}}},
	}))
	second, err := a.Latest(in, again.ID)
	require.NoError(t, err)
	require.NoError(t, a.RejectOnCancel(session.PrincipalInto(in, session.Principal{UserID: canceller, Source: session.SourceLocal}), second.ID, true))
	got, err = f.svcs.Approvals.Get(in, second.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.DeciderSystem, got.DecidedByKind, "a flag picked up by a drain is the system's, whoever drives the drain")
}

func TestRunApprovals_CallStates(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-x", "Org X", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, _ := seedThread(in, t, f, prj.ID, ag.ID)
	a := runApprovals{svcs: f.svcs}
	calls := []run.ApprovalCall{
		{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "pay", Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}},
		{ToolCallID: "call_2", InterruptID: "hook:beforeToolCall:call_2:oagent:approval", Server: "yasaku", Tool: "pay", Args: []byte(`{"amount":2}`), ArgsHash: "h2", Keys: []string{"amount"}},
	}
	require.NoError(t, a.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind, SnapshotHash: []byte{1}, Calls: calls,
	}))
	got, err := a.Latest(in, r.ID)
	require.NoError(t, err)
	_, err = f.svcs.Approvals.Decide(in, got.ID, approval.Answer{Approved: true, By: approval.Decider{Kind: approval.DeciderSystem}, Via: approval.ViaConsole})
	require.NoError(t, err)

	require.NoError(t, a.MarkExecuting(in, got.ID, []string{"call_1", "call_2"}))
	require.NoError(t, a.MarkExecuted(in, got.ID, "call_1"))
	require.NoError(t, a.MarkUnknown(in, got.ID, "call_2"))
	assert.Error(t, a.MarkExecuted(in, got.ID, "call_2"), "an unknown call never becomes executed")
	got, err = a.Latest(in, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "executed", got.Calls[0].State)
	assert.Equal(t, "unknown", got.Calls[1].State)
}

// SECURITY: only a real not-found is "gone"; the run expires an awaiting run on it, so the mapping must survive store changes.
func TestRunAdapters_NotFoundIsResourceGone(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-g", "Org G", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})

	_, err = runAgents{store: f.svcs.AgentStore, agents: f.svcs.Agents}.Version(in, uuid.New())
	require.ErrorIs(t, err, run.ErrResourceGone, "a missing pinned version")
	_, err = runServers{servers: f.svcs.MCPServers}.Open(in, uuid.New())
	require.ErrorIs(t, err, run.ErrResourceGone, "a missing granted MCP server")

	_, err = runServers{servers: f.svcs.MCPServers}.Open(context.Background(), uuid.New())
	require.Error(t, err)
	assert.NotErrorIs(t, err, run.ErrResourceGone, "any other error is retried, never read as gone")
}

type kickSpy struct{ threads []uuid.UUID }

func (k *kickSpy) Kick(_ context.Context, _, threadID uuid.UUID) error {
	k.threads = append(k.threads, threadID)
	return nil
}

// SECURITY: the POST that resumes holds the lease and runs the resumed run itself; a kick would let another instance race for it.
func TestAPIResume_DecidesAsTheCallerWithNoKick(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	o, err := org.NewOrg("org-d", "Org D", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, o.ID), o))
	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: o.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	in := tenant.Into(ctx, tenant.Context{OrgID: o.ID, ProjectID: prj.ID})
	th, r, _ := seedThread(in, t, f, prj.ID, ag.ID)
	require.NoError(t, runApprovals{svcs: f.svcs}.Raise(in, run.RaiseApproval{
		OrgID: o.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind, SnapshotHash: []byte{1},
		Calls: []run.ApprovalCall{{ToolCallID: "call_1", InterruptID: "hook:beforeToolCall:call_1:oagent:approval", Server: "yasaku", Tool: "pay",
			Args: []byte(`{"amount":1}`), ArgsHash: "h1", Keys: []string{"amount"}}},
	}))
	spy := &kickSpy{}
	dp := dataPlaneApprovals{approvals: f.svcs.Approvals.WithKicker(spy)}

	pending, err := dp.PendingForThread(in, th.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, []string{"call_1"}, pending.Open)
	key := uuid.New()
	require.NoError(t, dp.Decide(session.PrincipalInto(in, session.Principal{KeyID: key, Source: session.SourceAPIKey}), pending.ID, true, ""))
	got, err := f.svcs.Approvals.Get(in, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusApproved, got.Status)
	assert.Equal(t, approval.ViaAPI, got.DecidedVia)
	assert.Equal(t, key.String(), got.DecidedByID, "decided by the calling key")
	assert.Empty(t, spy.threads, "no kick: no other instance may pick the resumed run up")

	none, err := dp.PendingForThread(in, th.ID)
	require.NoError(t, err)
	assert.Nil(t, none, "a decided thread waits on nothing")
}
