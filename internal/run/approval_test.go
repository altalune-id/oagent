package run_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

// NOTE: awaitStream is the runtime's approval interrupt golden with one interrupt per tool call id.
func awaitStream(t *testing.T, calls ...string) []byte {
	t.Helper()
	return awaitWith(t, len(calls), func(i int, id *string, reason map[string]any) {
		*id = "hook:beforeToolCall:" + calls[i] + ":oagent:approval"
		reason["toolUseId"] = calls[i]
	})
}

// NOTE: awaitWith is the approval interrupt golden with n interrupts, each id and reason set by edit.
func awaitWith(t *testing.T, n int, edit func(i int, id *string, reason map[string]any)) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/approval_interrupt.stream.ndjson")
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	var term map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &term))
	one := term["interrupts"].([]any)[0].(map[string]any)
	var out []any
	for i := range n {
		reason := maps.Clone(one["reason"].(map[string]any))
		id := one["id"].(string)
		edit(i, &id, reason)
		out = append(out, map[string]any{"id": id, "name": one["name"], "reason": reason})
	}
	term["interrupts"] = out
	last, err := json.Marshal(term)
	require.NoError(t, err)
	lines[len(lines)-1] = last
	return append(bytes.Join(lines, []byte("\n")), '\n')
}

func (f *svcFixture) withApprovals() *fakes.RunApprovals {
	a := &fakes.RunApprovals{}
	f.svc.WithApprovals(a)
	return a
}

func (f *svcFixture) await(t *testing.T, a *fakes.RunApprovals, calls ...string) *run.Run {
	t.Helper()
	f.say(t, "record lunch 25000")
	r := f.execute(t, fakes.RuntimeReply{Stream: awaitStream(t, calls...)})
	require.Equal(t, run.StatusAwaitingApproval, r.Status)
	require.NotEmpty(t, a.RaisedSoFar())
	return r
}

func TestApproval_ParallelCallsOneRow(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	r := f.await(t, a, "call_1", "call_2")
	raised := a.RaisedSoFar()
	require.Len(t, raised, 1, "two gated calls in one step make one approval")
	in := raised[0].In
	assert.True(t, raised[0].InTx, "raised inside the settle's unit of work")
	assert.Equal(t, r.ID, in.RunID)
	assert.Equal(t, f.thread, in.ThreadID)
	require.Len(t, in.Calls, 2)
	assert.Equal(t, "call_1", in.Calls[0].ToolCallID)
	assert.Equal(t, "call_2", in.Calls[1].ToolCallID)
	assert.Equal(t, "hook:beforeToolCall:call_2:oagent:approval", in.Calls[1].InterruptID)
	assert.Equal(t, "yasaku", in.Calls[0].Server)
	assert.Equal(t, "record_expense", in.Calls[0].Tool)
	assert.Equal(t, []string{"amount", "account", "note", "confirm"}, in.Calls[0].Keys)
	assert.NotEmpty(t, in.Calls[0].ArgsHash)
	assert.JSONEq(t, `{"amount":25000,"account":"cash","note":"lunch","confirm":true}`, string(in.Calls[0].Args))
	snap, err := f.snaps.Load(f.ctx, f.thread)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, run.SnapshotHash(snap), in.SnapshotHash, "the hash of the snapshot this settle saved")
}

func TestRun_ParkedThreadRunsNoTurn(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	f.await(t, a, "call_1")
	calls := f.rt.Calls()
	f.say(t, "and another thing")
	r := f.execute(t)
	assert.Nil(t, r, "a pending approval parks the thread")
	assert.Equal(t, calls, f.rt.Calls(), "no turn reached the runtime")
	pending := 0
	for _, m := range f.store.Messages {
		if m.Status == run.MessagePending {
			pending++
		}
	}
	assert.Equal(t, 1, pending, "the new message waits")
}

func TestOpenRun_ReturnsDecidedAwaitingRun(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r)
	assert.Equal(t, parked.ID, r.ID, "the same run resumes")
	assert.Equal(t, parked.Attempt, r.Attempt, "with the same attempt count")
	assert.Equal(t, parked.AgentVersionID, r.AgentVersionID, "and its pinned version")
	assert.Equal(t, run.StatusCompleted, r.Status)
}

func TestResume_ApprovalAlwaysSendsTheSnapshot(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.NotNil(t, req.Snapshot, "an approval resume always sends the snapshot")
	assert.Empty(t, req.History)
	assert.Equal(t, runtime.InputResume, req.Input.Kind)
	require.Len(t, req.Input.Responses, 1)
	assert.Equal(t, "hook:beforeToolCall:call_1:oagent:approval", req.Input.Responses[0].InterruptID)
	assert.JSONEq(t, `{"approved":true,"text":"","argsHash":"4ef666461f69585461114208cacb71b083557e880445b98ed6ed3ba2918b138b"}`, string(req.Input.Responses[0].Response))
}

func TestResume_RejectTextReachesRuntime(t *testing.T) {
	for status, want := range map[string]string{"rejected": "no, it was 20000", "expired": "This approval expired"} {
		f := newSvc(t)
		a := f.withApprovals()
		parked := f.await(t, a, "call_1")
		a.Decide(parked.ID, status, "no, it was 20000")
		f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		var got struct {
			Approved bool   `json:"approved"`
			Text     string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(f.rt.LastRequest().Input.Responses[0].Response, &got), status)
		assert.False(t, got.Approved, status)
		assert.Equal(t, want, got.Text, status)
	}
}

func TestResume_CrashDuringApprovalResumeKeepsResponses(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	token := f.claim(t)
	_, _ = f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	require.Equal(t, run.StatusInterrupted, f.store.Runs[parked.ID].Status, "the resumed leg crashed")

	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	assert.NotNil(t, req.Snapshot, "the crash resume keeps the snapshot")
	assert.Equal(t, runtime.InputResume, req.Input.Kind, "and the same answers, never continue")
	require.Len(t, req.Input.Responses, 1)
	assert.Equal(t, "hook:beforeToolCall:call_1:oagent:approval", req.Input.Responses[0].InterruptID)
}

func TestResume_CrashUsesTheLatestApprovalOnly(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: awaitStream(t, "call_7")})
	require.Len(t, a.RaisedSoFar(), 2, "the resumed leg asked again")
	a.Decide(parked.ID, "approved", "")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	token := f.claim(t)
	_, _ = f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))

	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	resp := f.rt.LastRequest().Input.Responses
	require.Len(t, resp, 1)
	assert.Equal(t, "hook:beforeToolCall:call_7:oagent:approval", resp[0].InterruptID, "leg 3 answers the second interrupt, never the first")
}

func TestResume_SnapshotMismatchEndsExpired(t *testing.T) {
	for name, edit := range map[string]func(*run.Snapshot){
		"blob changed":                     func(s *run.Snapshot) { s.Blob = []byte(`{"appData":{"sdk":"1.23.0"},"changed":true}`) },
		"agent version changed, same blob": func(s *run.Snapshot) { s.AgentVersionID = uuid.New() },
		"row deleted":                      nil,
	} {
		f := newSvc(t)
		a := f.withApprovals()
		parked := f.await(t, a, "call_1")
		a.Decide(parked.ID, "approved", "")
		snap, err := f.snaps.Load(f.ctx, f.thread)
		require.NoError(t, err)
		if edit == nil {
			require.NoError(t, f.snaps.Delete(f.ctx, f.thread))
		} else {
			edit(snap)
			require.NoError(t, f.snaps.Save(f.ctx, snap))
		}
		calls := f.rt.Calls()
		r := f.execute(t)
		require.NotNil(t, r, name)
		assert.Equal(t, run.StatusExpired, r.Status, name)
		assert.Equal(t, calls, f.rt.Calls(), "%s: no answers are sent", name)
	}
}

func TestReply_AwaitingChannelRunSendsThePrompt(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	a := f.withApprovals()
	r := f.await(t, a, "call_1")
	assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState, "the prompt is due in the settle's own unit of work")
	assert.Equal(t, run.ReplyKindPrompt, f.stored(r).ReplyKind)

	api := newSvc(t)
	b := api.withApprovals()
	r = api.await(t, b, "call_1")
	assert.Empty(t, api.stored(r).ReplyState, "an API thread has no channel reply")
}

func TestResume_SnapshotLoadErrorRetriesNotExpires(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	calls := f.rt.Calls()
	f.snaps.Err = assert.AnError
	token := f.claim(t)
	_, err := f.svc.Execute(f.ctx, f.thread, token)
	require.Error(t, err, "a snapshot read error abandons the turn")
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	assert.Equal(t, run.StatusAwaitingApproval, f.store.Runs[parked.ID].Status, "a read error is not a missing snapshot: the run is not expired")
	assert.Equal(t, calls, f.rt.Calls())

	f.snaps.Err = nil
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r)
	assert.Equal(t, run.StatusCompleted, r.Status, "the retry resumes")
}

func TestResume_PastExpiryIsExpiredInTheStore(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Rows[parked.ID][0].ExpiresAt = time.Now().Add(-time.Minute)
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NotNil(t, r)
	assert.Equal(t, "expired", a.Rows[parked.ID][0].Status, "the row is expired in the store, so a later raise on the thread is not blocked")
	var got struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(f.rt.LastRequest().Input.Responses[0].Response, &got))
	assert.Equal(t, "This approval expired", got.Text)
}

func TestApproval_ReasonFieldsAreRead(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	f.say(t, "record lunch 25000")
	r := f.execute(t, fakes.RuntimeReply{Stream: awaitWith(t, 1, func(_ int, id *string, reason map[string]any) {
		*id = "an-opaque-strands-id"
		reason["toolUseId"], reason["server"], reason["tool"] = "call_7", "yasaku", "record_expense"
	})})
	require.Equal(t, run.StatusAwaitingApproval, r.Status, "Go reads the reason's fields and never parses the interrupt id")
	call := a.RaisedSoFar()[0].In.Calls[0]
	assert.Equal(t, "call_7", call.ToolCallID)
	assert.Equal(t, "an-opaque-strands-id", call.InterruptID, "the id is only echoed back on resume")
	assert.Equal(t, "yasaku", call.Server)
	assert.Equal(t, "record_expense", call.Tool)
}

func TestApproval_ReasonFailsClosed(t *testing.T) {
	type tc struct {
		n    int
		edit func(i int, id *string, reason map[string]any)
	}
	for name, c := range map[string]tc{
		"no toolUseId":       {1, func(_ int, _ *string, r map[string]any) { delete(r, "toolUseId") }},
		"empty toolUseId":    {1, func(_ int, _ *string, r map[string]any) { r["toolUseId"] = "" }},
		"no server":          {1, func(_ int, _ *string, r map[string]any) { delete(r, "server") }},
		"empty tool":         {1, func(_ int, _ *string, r map[string]any) { r["tool"] = "" }},
		"empty argsHash":     {1, func(_ int, _ *string, r map[string]any) { r["argsHash"] = "" }},
		"empty interrupt id": {1, func(_ int, id *string, _ map[string]any) { *id = "" }},
		"duplicate toolUseId": {2, func(i int, id *string, r map[string]any) {
			*id = "intr-" + string(rune('a'+i))
			r["toolUseId"] = "call_1"
		}},
		"duplicate interrupt id": {2, func(i int, id *string, r map[string]any) {
			*id = "intr-same"
			r["toolUseId"] = "call_" + string(rune('1'+i))
		}},
	} {
		f := newSvc(t)
		a := f.withApprovals()
		f.say(t, "record lunch 25000")
		r := f.execute(t, fakes.RuntimeReply{Stream: awaitWith(t, c.n, c.edit)})
		require.NotNil(t, r, name)
		assert.Equal(t, run.StatusFailed, r.Status, name)
		assert.Empty(t, a.RaisedSoFar(), "%s: nothing is raised", name)
	}
}
