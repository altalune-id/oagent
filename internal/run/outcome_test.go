package run_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestRelay_InterruptOutcomeNoStrandsID(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	f.say(t, "record lunch 25000")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: awaitStream(t, "call_1", "call_2")})
	token := f.claim(t)
	var events []runtime.Event
	_, err := f.svc.ExecuteObserved(f.ctx, f.thread, token, func(e runtime.Event) { events = append(events, e) })
	require.NoError(t, err)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	require.NotEmpty(t, a.RaisedSoFar())

	for _, e := range events {
		if strings.HasPrefix(string(e.EventType()), "OAGENT_") {
			continue
		}
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "hook:beforeToolCall", "no Strands interrupt id reaches a client event; OAGENT_ events never pass the relay")
	}

	fin, ok := events[len(events)-1].(*runtime.RunFinished)
	require.True(t, ok, "an awaiting run ends with RUN_FINISHED")
	var outcome struct {
		Type       string `json:"type"`
		Interrupts []struct {
			ID             string          `json:"id"`
			Reason         string          `json:"reason"`
			ToolCallID     string          `json:"toolCallId"`
			Message        string          `json:"message"`
			ExpiresAt      string          `json:"expiresAt"`
			ResponseSchema json.RawMessage `json:"responseSchema"`
			Metadata       struct {
				Approval string `json:"approval"`
			} `json:"metadata"`
		} `json:"interrupts"`
	}
	require.NoError(t, json.Unmarshal(fin.Outcome, &outcome))
	assert.Equal(t, "interrupt", outcome.Type)
	require.Len(t, outcome.Interrupts, 2, "one interrupt per gated call")
	latest := a.Rows[a.RaisedSoFar()[0].In.RunID][0]
	for i, want := range []string{"call_1", "call_2"} {
		in := outcome.Interrupts[i]
		assert.Equal(t, want, in.ID, "the id is the tool call id")
		assert.Equal(t, want, in.ToolCallID)
		assert.Equal(t, "tool_call", in.Reason)
		assert.Equal(t, latest.Preview, in.Message)
		assert.Equal(t, latest.PublicID, in.Metadata.Approval)
		assert.True(t, strings.HasSuffix(in.ExpiresAt, "Z"), in.ExpiresAt)
		assert.JSONEq(t, `{"type":"object","properties":{"approved":{"type":"boolean"},"text":{"type":"string"}},"required":["approved"]}`, string(in.ResponseSchema))
	}
}

func TestRun_OutcomeReadFailureNeverReadsAsSuccess(t *testing.T) {
	f := newSvc(t)
	a := f.withApprovals()
	a.LatestErr = assert.AnError
	f.say(t, "record lunch 25000")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
	token := f.claim(t)
	var events []runtime.Event
	_, err := f.svc.ExecuteObserved(f.ctx, f.thread, token, func(e runtime.Event) { events = append(events, e) })
	require.NoError(t, err, "the run is parked; only the outcome could not be read")
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	last := events[len(events)-1]
	_, finished := last.(*runtime.RunFinished)
	assert.False(t, finished, "a plain RUN_FINISHED would read as a completed run")
	runErr, ok := last.(*runtime.RunError)
	require.True(t, ok, "%T", last)
	assert.Equal(t, "awaiting_approval", runErr.Code)
}
