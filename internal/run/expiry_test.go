package run_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestRun_ExpiredWhenInterruptUnresumable(t *testing.T) {
	for name, gone := range map[string]func(c *room){
		"snapshot moved": func(c *room) {
			snap, err := c.f.snaps.Load(c.f.ctx, c.f.thread)
			require.NoError(t, err)
			snap.Blob = []byte(`{"changed":true}`)
			require.NoError(t, c.f.snaps.Save(c.f.ctx, snap))
		},
		"pinned version gone":     func(c *room) { delete(c.f.agents.Versions, c.parked.AgentVersionID) },
		"granted MCP server gone": func(c *room) { delete(c.f.servers.Servers, c.f.gated) },
	} {
		t.Run(name, func(t *testing.T) {
			c := newRoom(t, "dm")
			c.a.Decide(c.parked.ID, "approved", "")
			gone(c)
			calls := c.f.rt.Calls()
			token := c.f.claim(t)
			r, err := c.f.svc.Execute(c.f.ctx, c.f.thread, token)
			require.NoError(t, err, "an interrupt that can't resume ends the run, it is not retried")
			require.NoError(t, c.f.store.Release(c.f.ctx, c.f.thread, token))
			require.NotNil(t, r)
			assert.Equal(t, run.StatusExpired, r.Status)
			assert.Equal(t, calls, c.f.rt.Calls(), "no answers are sent")
			assert.Equal(t, "expired", c.a.Rows[c.parked.ID][0].Status, "the approval expires with the run")
			stored := c.f.stored(r)
			assert.Equal(t, run.ReplyDue, stored.ReplyState, "a DM is told")
			assert.Equal(t, run.ReplyKindExpired, stored.ReplyKind, "with the connection's approval_expired text")
		})
	}
}

func TestRun_ExpiredInAGroupSendsNoExpiredText(t *testing.T) {
	c := newRoom(t, "group")
	c.a.Decide(c.parked.ID, "approved", "")
	require.NoError(t, c.f.snaps.Delete(c.f.ctx, c.f.thread))
	r := c.drain(t)
	require.NotNil(t, r)
	assert.Equal(t, run.StatusExpired, r.Status)
	assert.NotEqual(t, run.ReplyKindExpired, c.f.stored(r).ReplyKind)
}

func TestApproval_GroupExpiryReleasesHeldMessages(t *testing.T) {
	c := newRoom(t, "group")
	held := c.post(t, c.member, "halo semua", "")
	c.a.Rows[c.parked.ID][0].ExpiresAt = time.Now().Add(-time.Second)
	r := c.drain(t)
	require.NotNil(t, r)
	assert.Equal(t, c.parked.ID, r.ID, "the parked run resumes with the expiry reject")
	assert.Equal(t, run.StatusCompleted, r.Status)
	next := c.drain(t)
	require.NotNil(t, next, "then the held messages run")
	m := c.message(held)
	require.NotNil(t, m.RunID)
	assert.Equal(t, next.ID, *m.RunID)
}

func TestCancel_AwaitingRejectsApproval(t *testing.T) {
	c := newRoom(t, "dm")
	require.NoError(t, c.f.svc.Cancel(c.f.ctx, c.parked.ID))
	assert.Equal(t, run.StatusCancelled, c.f.store.Runs[c.parked.ID].Status)
	assert.Equal(t, "rejected", c.a.Rows[c.parked.ID][0].Status, "the approval is rejected by the canceller")
	cancels := c.a.Cancelled()
	require.Len(t, cancels, 1)
	assert.True(t, cancels[0].InTx, "in the settle's unit of work")
	assert.False(t, cancels[0].BySystem, "a direct cancel is decided by the canceller")
}

func TestCancel_RequestedOnAParkedRunRejectsApproval(t *testing.T) {
	c := newRoom(t, "dm")
	require.NoError(t, c.f.store.RequestCancel(c.f.ctx, c.f.thread, c.parked.ID))
	r := c.drain(t)
	require.NotNil(t, r, "a cancel flag on a parked run is honoured, not parked")
	assert.Equal(t, run.StatusCancelled, r.Status)
	assert.Equal(t, "rejected", c.a.Rows[c.parked.ID][0].Status)
	cancels := c.a.Cancelled()
	require.Len(t, cancels, 1)
	assert.True(t, cancels[0].BySystem, "a cancel flag picked up by a drain is decided by the system, not by whoever drives the drain")
}

func TestCancel_ApprovedApprovalExpiresKeepingTheApprover(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	require.NoError(t, c.f.svc.Cancel(c.f.ctx, c.parked.ID))
	assert.Equal(t, run.StatusCancelled, c.f.store.Runs[c.parked.ID].Status)
	assert.Equal(t, "expired", c.a.Rows[c.parked.ID][0].Status, "approved calls that never ran are not left approved")
}

func TestResume_ReadErrorRetriesNeverExpires(t *testing.T) {
	for name, fail := range map[string]func(c *room){
		"version read error":    func(c *room) { c.f.agents.VersionErr = assert.AnError },
		"mcp server open error": func(c *room) { c.f.servers.OpenErr = assert.AnError },
	} {
		t.Run(name, func(t *testing.T) {
			c := newRoom(t, "dm")
			c.a.Decide(c.parked.ID, "approved", "")
			fail(c)
			calls := c.f.rt.Calls()
			token := c.f.claim(t)
			_, err := c.f.svc.Execute(c.f.ctx, c.f.thread, token)
			require.Error(t, err, "the turn is abandoned and retried")
			require.NoError(t, c.f.store.Release(c.f.ctx, c.f.thread, token))
			assert.Equal(t, run.StatusAwaitingApproval, c.f.store.Runs[c.parked.ID].Status, "never expired on a read error")
			assert.Equal(t, "approved", c.a.Rows[c.parked.ID][0].Status, "the approval is untouched")
			assert.Equal(t, calls, c.f.rt.Calls())

			c.f.agents.VersionErr, c.f.servers.OpenErr = nil, nil
			r := c.drain(t)
			require.NotNil(t, r)
			assert.Equal(t, run.StatusCompleted, r.Status, "the retry resumes")
		})
	}
}

func TestResume_SnapshotUnloadableExpires(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	calls := c.f.rt.Calls()
	r := c.f.execute(t, fakes.RuntimeReply{Err: &runtime.SnapshotUnloadableError{}}, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r)
	assert.Equal(t, run.StatusExpired, r.Status, "an approval resume never rebuilds without its snapshot")
	assert.Equal(t, calls+1, c.f.rt.Calls(), "no second turn with an empty agent")
	assert.Equal(t, "expired", c.a.Rows[c.parked.ID][0].Status, "the approval expires with the run")
	assert.Equal(t, run.ReplyKindExpired, c.f.stored(r).ReplyKind, "a DM is told")
}

func TestResume_FailedRunExpiresItsApproval(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(c *room)
		reply fakes.RuntimeReply
		code  apperror.Code
	}{
		"provider key cleared": {
			setup: func(c *room) {
				m := c.f.providers.Models[c.f.provider]["gpt-4o"]
				m.APIKey = ""
				c.f.providers.Models[c.f.provider]["gpt-4o"] = m
			},
			code: apperror.CodeModelProviderSecretRequired,
		},
		"runtime refuses the turn": {reply: fakes.RuntimeReply{Err: &runtime.StatusError{Status: 400}}, code: apperror.CodeRunRuntimeFailed},
		"attempts exhausted after an interrupted resume": {
			setup: func(c *room) {
				c.f.store.Runs[c.parked.ID].Status = run.StatusInterrupted
				c.f.store.Runs[c.parked.ID].Attempt = 3
			},
			code: apperror.CodeRunAttemptsExhausted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newRoom(t, "dm")
			c.a.Decide(c.parked.ID, "approved", "")
			if tc.setup != nil {
				tc.setup(c)
			}
			var replies []fakes.RuntimeReply
			if tc.reply.Err != nil {
				replies = append(replies, tc.reply)
			}
			r := c.f.execute(t, replies...)
			require.NotNil(t, r)
			assert.Equal(t, run.StatusFailed, r.Status)
			assert.Equal(t, string(tc.code), r.ErrorCode)
			assert.Equal(t, "expired", c.a.Rows[c.parked.ID][0].Status, "approved calls that never ran are not left approved")
		})
	}
}

func TestResume_ApprovedRunOnItsLastAttemptResumes(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	c.f.store.Runs[c.parked.ID].Attempt = 3
	r := c.drain(t)
	require.NotNil(t, r, "a run parked on its last attempt still resumes; the wait is not an attempt")
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, int32(3), r.Attempt)
}

func TestResume_FailedRunKeepsAnApprovalThatRan(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	row := &c.a.Rows[c.parked.ID][0]
	row.Calls[0].State = "executed"
	c.f.store.Runs[c.parked.ID].Status = run.StatusInterrupted
	c.f.store.Runs[c.parked.ID].Attempt = 3
	r := c.f.execute(t)
	require.NotNil(t, r)
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, "approved", c.a.Rows[c.parked.ID][0].Status, "a call ran, so the approval was used and stays approved")
}

func TestResume_FailedRunKeepsAnApprovalThatMayHaveRun(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	c.a.Rows[c.parked.ID][0].Calls[0].State = "unknown"
	c.f.store.Runs[c.parked.ID].Status = run.StatusInterrupted
	m := c.f.providers.Models[c.f.provider]["gpt-4o"]
	m.APIKey = ""
	c.f.providers.Models[c.f.provider]["gpt-4o"] = m
	r := c.f.execute(t)
	require.NotNil(t, r)
	assert.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, "approved", c.a.Rows[c.parked.ID][0].Status, "an unknown call may have run, so the approval stays approved")
}

func TestResume_SnapshotUnloadableAfterACallRanFails(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	c.a.Rows[c.parked.ID][0].Calls[0].State = "executed"
	c.f.store.Runs[c.parked.ID].Status = run.StatusInterrupted
	r := c.f.execute(t, fakes.RuntimeReply{Err: &runtime.SnapshotUnloadableError{}})
	require.NotNil(t, r)
	assert.Equal(t, run.StatusFailed, r.Status, "a run whose call ran never ends expired")
	assert.Equal(t, "approved", c.a.Rows[c.parked.ID][0].Status, "the used approval stays approved")
	assert.NotEqual(t, run.ReplyKindExpired, c.f.stored(r).ReplyKind, "no 'please ask again' after a side effect")
}

func TestResume_AttemptsExhaustedSettlesExecutingCalls(t *testing.T) {
	c := newRoom(t, "dm")
	c.a.Decide(c.parked.ID, "approved", "")
	c.a.Rows[c.parked.ID][0].Calls[0].State = "executing"
	c.f.store.Runs[c.parked.ID].Status = run.StatusInterrupted
	c.f.store.Runs[c.parked.ID].Attempt = 3
	r := c.f.execute(t)
	require.NotNil(t, r)
	assert.Equal(t, run.StatusFailed, r.Status)
	row := c.a.Rows[c.parked.ID][0]
	assert.Equal(t, "unknown", row.Calls[0].State, "no call stays executing on a failed run")
	assert.Equal(t, "approved", row.Status)
}
