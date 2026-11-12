package approval_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"altalune.id/oagent/apperror"
	approvalv1 "altalune.id/oagent/gen/go/approval/v1"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/cptest"
)

type names struct{}

func (names) EndUser(_ context.Context, id uuid.UUID) (string, string, error) {
	return "EuSari2wE4rT6yU8jP9pA", "Sari Wulandari", nil
}
func (names) Member(context.Context, uuid.UUID) (string, error)  { return "Dina Admin", nil }
func (names) KeyHint(context.Context, uuid.UUID) (string, error) { return "7Kq2", nil }

type cpWorld struct {
	*world
	cp       *approval.ApprovalService
	member   uuid.UUID
	otherAg  uuid.UUID
	pending  *approval.Approval
	otherApr *approval.Approval
	apiApr   *approval.Approval
}

func newCPWorld(t *testing.T) *cpWorld {
	t.Helper()
	w := &cpWorld{world: newWorld(t), member: uuid.New(), otherAg: uuid.New()}
	w.ids.User(w.member)
	w.ids.Resource(w.otherAg, w.org)
	w.cp = approval.NewControlPlane(w.svc, w.refs, names{})
	sari := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	w.pending = w.create(t, "dm", sari)
	other, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.otherAg, RunID: w.run, ThreadID: uuid.New(), ThreadKind: "group",
		Calls: []approval.Call{expenseCall()},
	})
	require.NoError(t, err)
	w.otherApr = other
	return w
}

func (w *cpWorld) withAPIApproval(t *testing.T) {
	t.Helper()
	a, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: w.run, ThreadID: uuid.New(), ThreadKind: "api",
		Calls: []approval.Call{expenseCall()},
	})
	require.NoError(t, err)
	w.apiApr = a
}

func (w *cpWorld) person() context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceLocal, UserID: w.member, ActiveOrgID: w.org, Scopes: []string{"*"},
	})
}

func (w *cpWorld) agentKey(agent uuid.UUID) context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: w.org, ProjectIDs: []uuid.UUID{w.project}, ResourceIDs: []uuid.UUID{agent},
	})
}

func TestApprovalReach_IDOnlyRPCs(t *testing.T) {
	w := newCPWorld(t)
	stranger := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: uuid.New(), Scopes: []string{"*"}})
	for name, ctx := range map[string]context.Context{"another org's member": stranger, "a key for the other agent": w.agentKey(w.otherAg)} {
		_, err := w.cp.GetApproval(ctx, connect.NewRequest(&approvalv1.GetApprovalRequest{Id: w.pending.PublicID}))
		assert.True(t, approval.IsNotFoundError(err), "%s get: %v", name, err)
		_, err = w.cp.DecideApproval(ctx, connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.pending.PublicID, Approved: true}))
		assert.True(t, approval.IsNotFoundError(err), "%s decide: %v", name, err)
	}
	_, err := w.cp.ListApprovals(stranger, connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub}))
	assert.True(t, approval.IsNotFoundError(err), "another org's project")
	_, err = w.cp.GetApproval(w.person(), connect.NewRequest(&approvalv1.GetApprovalRequest{Id: "unknwnunknwnunknwnabc"}))
	assert.True(t, approval.IsNotFoundError(err), "an unknown approval answers the same")
	got, err := w.store.Get(w.ctx, w.pending.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusPending, got.Status, "a refused decision changes nothing")
	_, err = w.cp.GetApproval(w.agentKey(w.agent), connect.NewRequest(&approvalv1.GetApprovalRequest{Id: w.pending.PublicID}))
	assert.NoError(t, err, "a key for the approval's agent reads it")
}

func TestListApprovals_PendingCountIsWhatTheCallerReaches(t *testing.T) {
	w := newCPWorld(t)
	all, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	assert.Len(t, all.Msg.GetApprovals(), 2)
	assert.Equal(t, int64(2), all.Msg.GetPendingCount())

	mine, err := w.cp.ListApprovals(w.agentKey(w.agent), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	require.Len(t, mine.Msg.GetApprovals(), 1, "an agent-scoped key lists only its agent's approvals")
	assert.Equal(t, w.pending.PublicID, mine.Msg.GetApprovals()[0].GetId())
	assert.Equal(t, int64(1), mine.Msg.GetPendingCount(), "and counts only those")

	decided, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, Status: "approved"}))
	require.NoError(t, err)
	assert.Empty(t, decided.Msg.GetApprovals())
	assert.Equal(t, int64(2), decided.Msg.GetPendingCount(), "the count ignores the filters")

	none, err := w.cp.ListApprovals(w.agentKey(w.agent), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, AgentId: w.ids.Resource(w.otherAg, w.org)}))
	require.NoError(t, err)
	assert.Empty(t, none.Msg.GetApprovals(), "asking for an agent out of reach lists nothing")

	page, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, PageSize: 1}))
	require.NoError(t, err)
	require.NotEmpty(t, page.Msg.GetNextPageToken())
	next, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, PageToken: page.Msg.GetNextPageToken()}))
	require.NoError(t, err)
	assert.Len(t, next.Msg.GetApprovals(), 1)
	_, err = w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, PageToken: "%%%"}))
	var bad *approval.InvalidPageTokenError
	assert.ErrorAs(t, err, &bad)
}

func TestDecideApproval_SamePathAsWhatsApp(t *testing.T) {
	w := newCPWorld(t)
	before := len(w.pub.Events())
	resp, err := w.cp.DecideApproval(w.person(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.pending.PublicID, Approved: false, Text: "no, it was 20000"}))
	require.NoError(t, err)
	got := resp.Msg.GetApproval()
	assert.Equal(t, "rejected", got.GetStatus())
	assert.Equal(t, "no, it was 20000", got.GetDecisionText())
	assert.Equal(t, w.ids.User(w.member), got.GetDecidedBy(), "the member's public id")
	assert.Equal(t, "Dina Admin", got.GetDecidedByLabel())
	assert.Equal(t, "mcp", got.GetDecidedVia(), "an in-process call is the MCP tool's")
	assert.NotNil(t, got.GetDecidedAt())

	evs := w.pub.Events()[before:]
	require.Len(t, evs, 1, "the domain decision emits approval.decided")
	assert.Equal(t, events.ApprovalDecided, evs[0].Type)
	assert.Equal(t, []uuid.UUID{w.thread}, w.kicks.threads, "a console or MCP decision wakes the run, like WhatsApp")

	_, err = w.cp.DecideApproval(w.person(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.pending.PublicID, Approved: true}))
	var decided *approval.AlreadyDecidedError
	require.ErrorAs(t, err, &decided, "single use through the handler too")
	assert.Equal(t, "rejected", decided.Status)

	key := w.agentKey(w.otherAg)
	resp, err = w.cp.DecideApproval(key, connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.otherApr.PublicID, Approved: true}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetApproval().GetDecidedBy(), "an API decision names no member")
	assert.Equal(t, "…7Kq2", resp.Msg.GetApproval().GetDecidedByLabel())
}

// SECURITY: the API shows the requester as a label and public id, and the preview as plain text with no WhatsApp escapes.
func TestGetApproval_LabelsAndPlainPreview(t *testing.T) {
	w := newCPWorld(t)
	resp, err := w.cp.GetApproval(w.person(), connect.NewRequest(&approvalv1.GetApprovalRequest{Id: w.pending.PublicID}))
	require.NoError(t, err)
	a := resp.Msg.GetApproval()
	assert.Equal(t, "EuSari2wE4rT6yU8jP9pA", a.GetRequesterEndUserId())
	assert.Equal(t, "Sari Wulandari", a.GetRequesterLabel())
	assert.Empty(t, a.GetDecidedBy())
	assert.Empty(t, a.GetDecidedByLabel(), "nobody has decided yet")
	assert.Equal(t, []string{"record_expense"}, a.GetTools())
	assert.Equal(t, "whatsapp", a.GetChannel())
	assert.Equal(t, w.agentPub, a.GetAgentId())
	assert.Equal(t, w.threadPub, a.GetThreadId())
	assert.Equal(t, w.runPub, a.GetRunId())
	assert.Equal(t, "yasaku · record_expense\namount: 25000\nnote: *kopi* _susu_\nconfirm: true", a.GetPreview())
	assert.NotContains(t, a.GetPreview(), "\u2060")
	body := protojson.Format(resp.Msg)
	for _, id := range []uuid.UUID{w.org, w.project, w.agent, w.thread, w.run, w.pending.ID} {
		assert.NotContains(t, body, id.String(), "public ids only")
	}
	assert.False(t, strings.Contains(body, `"args"`), "no raw arguments leave the server")
}

func TestDecideApproval_RefusesAnAPIConversationsApproval(t *testing.T) {
	w := newCPWorld(t)
	w.withAPIApproval(t)
	_, err := w.cp.DecideApproval(w.person(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.apiApr.PublicID, Approved: true}))
	require.True(t, approval.IsAnsweredByAppError(err), "%v", err)
	assert.Equal(t, apperror.CodeApprovalAnsweredByApp, (&approval.AnsweredByAppError{}).ToAppError().Code())
	got, err := w.store.Get(w.ctx, w.apiApr.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusPending, got.Status, "the refusal decides nothing")
	assert.Empty(t, w.kicks.threads)
	read, err := w.cp.GetApproval(w.person(), connect.NewRequest(&approvalv1.GetApprovalRequest{Id: w.apiApr.PublicID}))
	require.NoError(t, err, "the console still reads it")
	assert.Equal(t, "api", read.Msg.GetApproval().GetChannel())
	assert.Empty(t, read.Msg.GetApproval().GetDecidedVia(), "unset while pending")
}

func TestDecideApproval_ThroughConnectIsTheConsole(t *testing.T) {
	w := newCPWorld(t)
	srv := controlplane.New(controlplane.Services{Approvals: w.svc, ApprovalRefs: w.refs, ApprovalNames: names{}}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	client := apiclient.New(ts.URL, cptest.OperatorCredential()).Approvals
	resp, err := client.DecideApproval(t.Context(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.pending.PublicID, Approved: true}))
	require.NoError(t, err)
	assert.Equal(t, "console", resp.Msg.GetApproval().GetDecidedVia())
	stored, err := w.store.Get(w.ctx, w.pending.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.ViaConsole, stored.DecidedVia, "stored with the decision")
}

func TestListApprovals_ExpiredIsNotPending(t *testing.T) {
	w := newCPWorld(t)
	w.now = w.now.Add(10 * time.Minute)
	resp, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, Status: "pending"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetApprovals(), 1, "the group approval expired at 10 minutes, before the sweep runs")
	assert.Equal(t, w.pending.PublicID, resp.Msg.GetApprovals()[0].GetId())
	assert.Equal(t, int64(1), resp.Msg.GetPendingCount())
}

type countingNames struct {
	names
	mu    sync.Mutex
	calls int
}

func (c *countingNames) EndUser(ctx context.Context, id uuid.UUID) (string, string, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.names.EndUser(ctx, id)
}

func TestListApprovals_NamesEachPersonOncePerPage(t *testing.T) {
	w := newCPWorld(t)
	sari := w.pending.Requester
	for range 3 {
		_, err := w.svc.Create(w.ctx, approval.CreateInput{
			OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: w.run, ThreadID: uuid.New(), ThreadKind: "dm",
			Calls: []approval.Call{expenseCall()}, Speakers: []uuid.NullUUID{sari},
		})
		require.NoError(t, err)
	}
	counted := &countingNames{}
	cp := approval.NewControlPlane(w.svc, w.refs, counted)
	resp, err := cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetApprovals(), 5)
	assert.Equal(t, 1, counted.calls, "four approvals by Sari, one lookup")
}

func TestListApprovals_DecidedMeansApprovedOrRejected(t *testing.T) {
	w := newCPWorld(t)
	_, err := w.cp.DecideApproval(w.person(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.pending.PublicID, Approved: false}))
	require.NoError(t, err)
	_, err = w.cp.DecideApproval(w.person(), connect.NewRequest(&approvalv1.DecideApprovalRequest{Id: w.otherApr.PublicID, Approved: true}))
	require.NoError(t, err)
	w.withAPIApproval(t)
	resp, err := w.cp.ListApprovals(w.person(), connect.NewRequest(&approvalv1.ListApprovalsRequest{ProjectId: w.projectPub, Status: "decided"}))
	require.NoError(t, err)
	got := map[string]string{}
	for _, a := range resp.Msg.GetApprovals() {
		got[a.GetId()] = a.GetStatus()
	}
	assert.Equal(t, map[string]string{w.pending.PublicID: "rejected", w.otherApr.PublicID: "approved"}, got, "the pending one is left out")
}
