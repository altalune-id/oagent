package dataplane_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/dataplane"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type approvals struct {
	ra      *fakes.RunApprovals
	store   *fakes.RunStore
	mu      sync.Mutex
	decided []bool
}

func (a *approvals) PendingForThread(ctx context.Context, threadID uuid.UUID) (*dataplane.PendingApproval, error) {
	for _, r := range a.store.AllRuns() {
		if r.ThreadID != threadID {
			continue
		}
		got, err := a.ra.Latest(ctx, r.ID)
		if err != nil || got == nil || got.Status != "pending" {
			continue
		}
		out := &dataplane.PendingApproval{ID: got.ID, PublicID: got.PublicID}
		for _, c := range got.Calls {
			out.Open = append(out.Open, c.ToolCallID)
		}
		return out, nil
	}
	return nil, nil
}

func (a *approvals) Decide(_ context.Context, id uuid.UUID, approved bool, text string) error {
	for runID, rows := range a.ra.Rows {
		if rows[len(rows)-1].ID == id {
			status := "rejected"
			if approved {
				status = "approved"
			}
			a.ra.Decide(runID, status, text)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.decided = append(a.decided, approved)
	return nil
}

func approvalStream(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "run", "testdata", "approval_interrupt.stream.ndjson"))
	require.NoError(t, err)
	return raw
}

// NOTE: parked posts a turn that parks its thread on an approval, as actor alice through the decide key.
func parked(t *testing.T) (*plane, *approvals) {
	t.Helper()
	ap := &approvals{ra: &fakes.RunApprovals{}}
	p := newPlane(t, 4, func(d *dataplane.Deps) { d.Approvals = ap })
	ap.store = p.store
	p.svc.WithApprovals(ap.ra)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: approvalStream(t)}}
	resp := p.post(t, decideKey, "AGENTpub", body(clientThread, "record lunch 25000"), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Body, `"outcome":{"type":"interrupt"`, "the POST ends with the interrupt outcome")
	require.Eventually(t, func() bool {
		runs := p.store.AllRuns()
		return len(runs) == 1 && runs[0].Status == run.StatusAwaitingApproval && p.store.LeaseOwner(firstThread(p)) == ""
	}, 5e9, 1e7)
	return p, ap
}

func resumeBody(entries string) string {
	return `{"threadId":"` + clientThread + `","messages":[],"resume":` + entries + `}`
}

const approveAll = `[{"interruptId":"call_1","status":"resolved","payload":{"approved":true}}]`

func TestPOSTRuns_PendingApprovalIs409(t *testing.T) {
	p, _ := parked(t)
	messages := len(p.store.Messages)
	resp := p.post(t, decideKey, "AGENTpub", body(clientThread, "and another"), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	code, meta := errorCode(t, resp)
	assert.Equal(t, apperror.CodeApprovalPending, code)
	assert.True(t, strings.HasPrefix(meta["approval"], "apr_"), meta)
	assert.Len(t, p.store.Messages, messages, "a refused POST leaves no rows")
}

func TestDecide_NoApprovalScopeIs403(t *testing.T) {
	p, ap := parked(t)
	resp := p.post(t, orgKey, "AGENTpub", resumeBody(approveAll), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	code, meta := errorCode(t, resp)
	assert.Equal(t, apperror.CodeForbidden, code)
	assert.Equal(t, "approval:decide", meta["permission"])
	assert.Empty(t, ap.decided)
}

func TestDecide_KeyBoundToThreadUser(t *testing.T) {
	p, ap := parked(t)
	_, err := p.eu.UpsertAPIUser(t.Context(), p.project, "bob")
	require.NoError(t, err, "bob is a known end user of the project, just not this thread's")
	resp := p.post(t, decideKey, "AGENTpub", resumeBody(approveAll), dataplane.UserHeader, "bob")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "another actor's thread is not found")
	assert.Empty(t, ap.decided)
}

func TestResume_StreamsItsOwnResumedRun(t *testing.T) {
	p, ap := parked(t)
	parkedRun := p.store.AllRuns()[0]
	p.rt.Replies = append(p.rt.Replies, fakes.RuntimeReply{Stream: runtimeStream(t, "plain_reply")})
	resp := p.post(t, decideKey, "AGENTpub", resumeBody(approveAll), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusOK, resp.StatusCode, resp.Body)
	assert.Contains(t, resp.Body, "RUN_FINISHED", "the POST streams the resumed run")
	assert.Equal(t, []bool{true}, ap.decided)
	r := p.settled(t)
	assert.Equal(t, parkedRun.ID, r.ID, "the parked run resumed")
	assert.Equal(t, run.StatusCompleted, r.Status)
}

func TestResume_IncompleteIs400Here(t *testing.T) {
	p, ap := parked(t)
	resp := p.post(t, decideKey, "AGENTpub", resumeBody(`[]`), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	code, meta := errorCode(t, resp)
	assert.Equal(t, apperror.CodeResumeIncomplete, code)
	assert.Equal(t, "call_1", meta["missing"])
	assert.Empty(t, ap.decided)
}

func TestResume_ClaimsTheLeaseBeforeItDecides(t *testing.T) {
	p, ap := parked(t)
	thread := firstThread(p)
	claim, err := p.store.Claim(tenant.Into(context.Background(), tenant.Context{OrgID: p.org, ProjectID: p.project}), thread, "another-instance", 5e9)
	require.NoError(t, err)
	require.Equal(t, run.Claimed, claim)
	resp := p.post(t, decideKey, "AGENTpub", resumeBody(approveAll), dataplane.UserHeader, "alice")
	require.Equal(t, http.StatusConflict, resp.StatusCode, resp.Body)
	code, _ := errorCode(t, resp)
	assert.Equal(t, apperror.CodeRunInProgress, code)
	assert.Empty(t, ap.decided, "nothing is decided while another instance holds the thread")
}
