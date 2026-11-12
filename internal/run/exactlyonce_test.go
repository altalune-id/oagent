package run_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

// NOTE: before keeps the stream lines before the first line containing marker, as a crash there would.
func before(raw []byte, marker string) []byte {
	var out []byte
	for _, line := range bytes.SplitAfter(raw, []byte("\n")) {
		if bytes.Contains(line, []byte(marker)) {
			break
		}
		out = append(out, line...)
	}
	return out
}

// NOTE: crashedLeg approves the parked run, then lets its resume leg stream raw and crash.
func crashedLeg(t *testing.T, raw []byte) (*svcFixture, *fakes.RunApprovals, *run.Run) {
	t.Helper()
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: raw})
	token := f.claim(t)
	_, _ = f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	require.Equal(t, run.StatusInterrupted, f.store.Runs[parked.ID].Status, "the resume leg crashed")
	return f, a, parked
}

func TestResume_StoredResultNeverRunsAgain(t *testing.T) {
	f, a, parked := crashedLeg(t, cut(stream(t, "resume")))
	assert.Equal(t, "executed", a.CallState(parked.ID, "call_1"), "the checkpoint marks the call executed")

	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.Len(t, req.StoredResults, 1, "the crash resume replays the stored result")
	assert.Equal(t, "call_1", req.StoredResults[0].ToolUseID)
	assert.Contains(t, string(req.StoredResults[0].Result), `"status":"success"`)
	assert.Equal(t, "executed", a.CallState(parked.ID, "call_1"))
}

func TestResume_ExecutingWithoutResultNotRerun(t *testing.T) {
	f, a, parked := crashedLeg(t, before(stream(t, "resume"), "TOOL_CALL_RESULT"))
	assert.Equal(t, "executing", a.CallState(parked.ID, "call_1"), "marked executing and committed before the request")

	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, "unknown", a.CallState(parked.ID, "call_1"), "never run again")
}

func TestResume_HashMismatchRefused(t *testing.T) {
	for state, approved := range map[string]bool{"approved": true, "executed": true, "executing": true, "unknown": false, "": false} {
		ap := &run.RunApproval{Status: "approved", Calls: []run.ApprovalCall{{State: state, InterruptID: "hook:beforeToolCall:call_1:oagent:approval", ArgsHash: "row-hash"}}}
		got, err := run.ResumeResponses(ap)
		require.NoError(t, err)
		var body struct {
			Approved bool   `json:"approved"`
			ArgsHash string `json:"argsHash"`
		}
		require.NoError(t, json.Unmarshal(got[0].Response, &body))
		assert.Equal(t, approved, body.Approved, "a call in state %q", state)
		assert.Equal(t, "row-hash", body.ArgsHash, "the row's hash, never one Go computes; the runtime denies a call whose live args hash differently")
	}
}

func TestResume_UnknownCallAnswersTheUnknownText(t *testing.T) {
	f, a, parked := crashedLeg(t, before(stream(t, "resume"), "TOOL_CALL_RESULT"))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, "unknown", a.CallState(parked.ID, "call_1"))
	req := f.rt.LastRequest()
	require.Len(t, req.Input.Responses, 1)
	var got struct {
		Approved bool   `json:"approved"`
		Text     string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(req.Input.Responses[0].Response, &got))
	assert.False(t, got.Approved)
	assert.Equal(t, "the outcome of this call is unknown after a restart; check before trying again", got.Text,
		"the runtime denies with the answer's text before any replay, so the model must read why")
	assert.Empty(t, req.StoredResults, "no stored result for an unknown call: the denial answers it")
}

func TestResume_ExecutingAtTurnTime(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	var atTurn string
	f.rt.BeforeTurn = func() { atTurn = a.CallState(parked.ID, "call_1") }
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume")})
	assert.Equal(t, "executing", atTurn, "committed before the request leaves Go")
}

func TestResume_UndeliveredRequestRetriesAndRunsOnce(t *testing.T) {
	for name, undelivered := range map[string]error{
		"dial error": &url.Error{Op: "Post", URL: "http://runtime/turn", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
		"/turn 503":  &runtime.BusyError{},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSvc(t)
			a := f.withApprovals()
			parked := f.await(t, a, "call_1")
			a.Decide(parked.ID, "approved", "")
			f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Err: undelivered})
			token := f.claim(t)
			_, _ = f.svc.Execute(f.ctx, f.thread, token)
			require.NoError(t, f.store.Release(f.ctx, f.thread, token))
			assert.Equal(t, "approved", a.CallState(parked.ID, "call_1"), "a request never delivered puts the call back to approved")

			r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume")})
			require.NotNil(t, r)
			assert.Equal(t, run.StatusCompleted, r.Status)
			assert.Equal(t, "executed", a.CallState(parked.ID, "call_1"), "runs once on the retry, never unknown")
			var got struct {
				Approved bool `json:"approved"`
			}
			require.NoError(t, json.Unmarshal(f.rt.LastRequest().Input.Responses[0].Response, &got))
			assert.True(t, got.Approved)
		})
	}
}

func TestResume_StaleLeaseAtTheCommitSendsNoTurn(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	calls := f.rt.Calls()
	f.store.LeaseStolen = true
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	require.Error(t, err)
	assert.Equal(t, calls, f.rt.Calls(), "no Turn without the lease")
	assert.Equal(t, "approved", a.CallState(parked.ID, "call_1"), "nothing marked executing")
}

func TestResume_MaybeDeliveredStaysExecutingThenUnknown(t *testing.T) {
	for name, maybe := range map[string]error{
		"504":                 &runtime.StatusError{Status: 504},
		"502":                 &runtime.StatusError{Status: 502},
		"read timeout":        &url.Error{Op: "Post", URL: "http://runtime/turn", Err: os.ErrDeadlineExceeded},
		"reset after headers": &url.Error{Op: "Post", URL: "http://runtime/turn", Err: syscall.ECONNRESET},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSvc(t)
			a := f.withApprovals()
			parked := f.await(t, a, "call_1")
			a.Decide(parked.ID, "approved", "")
			f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Err: maybe})
			token := f.claim(t)
			_, _ = f.svc.Execute(f.ctx, f.thread, token)
			require.NoError(t, f.store.Release(f.ctx, f.thread, token))
			assert.Equal(t, "executing", a.CallState(parked.ID, "call_1"), "the request may have reached the runtime")

			f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			assert.Equal(t, "unknown", a.CallState(parked.ID, "call_1"), "never run again")
		})
	}
}

func TestResume_GatewayErrorFailsTheRunWithTheCallNeverRerun(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Err: &runtime.StatusError{Status: 500}})
	r := f.execute(t)
	require.NotNil(t, r)
	assert.Equal(t, run.StatusFailed, r.Status, "a 500 fails the run")
	assert.Equal(t, "executing", a.CallState(parked.ID, "call_1"), "the request may have run, so the call is never put back to approved")
}
