package run_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

func (f *svcFixture) consumed(msgs ...run.Message) {
	for _, m := range msgs {
		m.ThreadID, m.Status = f.thread, run.MessageConsumed
		f.store.Messages = append(f.store.Messages, m)
	}
}

func assistantCall(callID string) run.Message {
	body, _ := json.Marshal(map[string]any{
		"id": uuid.NewString(), "role": "assistant",
		"toolCalls": []map[string]any{{"id": callID, "type": "function", "function": map[string]string{"name": "yasaku__record_expense", "arguments": "{}"}}},
	})
	return run.Message{Role: run.RoleAssistant, Body: body}
}

func roles(t *testing.T, history []json.RawMessage) []string {
	t.Helper()
	out := make([]string, len(history))
	for i, h := range history {
		var m struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"toolCallId"`
		}
		require.NoError(t, json.Unmarshal(h, &m))
		out[i] = m.Role
	}
	return out
}

func TestRun_HistoryBoundedToFortyMessages(t *testing.T) {
	f := newSvc(t)
	for range 30 {
		f.consumed(userMsg("earlier", ""), assistantMsg("ok"))
	}
	f.consumed(userMsg("newest earlier ask", ""), assistantMsg("newest earlier answer"))
	f.say(t, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	h := f.rt.LastRequest().History
	require.NotEmpty(t, h)
	assert.LessOrEqual(t, len(h), 40)
	assert.Equal(t, "user", roles(t, h)[0], "the window starts on a user row")
	assert.Contains(t, string(h[len(h)-1]), "newest earlier answer", "the newest rows are kept")
}

func TestRun_HistoryNeverOrphansAToolResult(t *testing.T) {
	f := newSvc(t)
	f.consumed(userMsg("old ask", ""), assistantCall("call_old"), toolMsg("call_old"))
	for range 19 {
		f.consumed(userMsg("earlier", ""), assistantMsg("ok"))
	}
	f.say(t, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	h := f.rt.LastRequest().History
	require.NotEmpty(t, h)
	assert.Equal(t, "user", roles(t, h)[0], "a cut between a call and its result starts at the next user row")
	for _, raw := range h {
		assert.NotContains(t, string(raw), "call_old", "the old tool round is dropped whole")
	}
}

func TestRun_LongSingleTurnSentWhole(t *testing.T) {
	f := newSvc(t)
	f.consumed(userMsg("earlier", ""), assistantMsg("ok"))
	f.consumed(userMsg("the long ask", ""))
	for i := range 25 {
		id := "call_" + string(rune('a'+i))
		f.consumed(assistantCall(id), toolMsg(id))
	}
	f.say(t, "hello")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	h := f.rt.LastRequest().History
	require.Len(t, h, 51, "a turn longer than the window is sent whole from its user row")
	assert.Contains(t, string(h[0]), "the long ask")
}

func TestRun_ContinueHistoryEndsOnTheToolRow(t *testing.T) {
	f := newSvc(t)
	for range 30 {
		f.consumed(userMsg("earlier", ""), assistantMsg("ok"))
	}
	f.say(t, "record lunch")
	f.execute(t, fakes.RuntimeReply{Stream: cut(stream(t, "tool_call"))})
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume_after_crash")})
	req := f.rt.LastRequest()
	require.NotEmpty(t, req.History)
	assert.LessOrEqual(t, len(req.History), 40)
	r := roles(t, req.History)
	assert.Equal(t, "user", r[0])
	assert.Equal(t, "tool", r[len(r)-1], "a crash continue still ends on its tool round")
}
