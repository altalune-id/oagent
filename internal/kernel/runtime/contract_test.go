package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var goldenCases = []string{
	"plain_reply", "tool_call", "mcp_server_down", "resume", "rebuild_from_history", "stored_result", "limit_reached", "cancel", "model_error", "resume_after_crash",
	"image_input", "image_refused", "anthropic_reply",
	"approval_interrupt", "approval_rule_broken", "approval_resume_approved", "approval_resume_rejected", "approval_hash_mismatch", "approval_absent_only",
}

var goldenVersions = map[string]int{
	"approval_interrupt": 3, "approval_rule_broken": 3, "approval_resume_approved": 3, "approval_resume_rejected": 3, "approval_hash_mismatch": 3,
	"approval_absent_only": 3,
}

func goldenVersion(name string) int {
	if v, ok := goldenVersions[name]; ok {
		return v
	}
	return ContractVersion
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "turn", name))
	require.NoError(t, err)
	return raw
}

func streamLines(t *testing.T, raw []byte) [][]byte {
	t.Helper()
	var lines [][]byte
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	require.NoError(t, sc.Err())
	return lines
}

func TestGoldens_DecodeEveryCase(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "turn", "*"))
	require.NoError(t, err)
	assert.Len(t, files, 2*len(goldenCases), "a request and a stream per case, nothing else")

	for _, c := range goldenCases {
		t.Run(c, func(t *testing.T) {
			req, err := DecodeRequest(readGolden(t, c+".request.json"))
			require.NoError(t, err, "request")
			assert.Equal(t, goldenVersion(c), req.ContractVersion)
			for _, sr := range req.StoredResults {
				assertToolResult(t, sr.Result)
			}
			for _, m := range req.History {
				assertAGUIMessage(t, m)
			}

			lines := streamLines(t, readGolden(t, c+".stream.ndjson"))
			require.NotEmpty(t, lines)
			terminals := 0
			for i, line := range lines {
				ev, err := DecodeEvent(line)
				require.NoError(t, err, "line %d: %s", i+1, line)
				if term, ok := ev.(*Terminal); ok {
					terminals++
					assert.Equal(t, len(lines)-1, i, "the terminal is the last line")
					assert.True(t, slices.Contains(terminalStatuses, term.Status), "status %q", term.Status)
					assert.Equal(t, term.Status == StatusFailed, string(term.Snapshot) == "null", "only a failed turn has no snapshot")
					for _, m := range term.MessagesAppended {
						assertAGUIMessage(t, m)
					}
				}
				if cp, ok := ev.(*Checkpoint); ok {
					assertToolResult(t, cp.Result)
				}
			}
			assert.Equal(t, 1, terminals, "exactly one terminal")
		})
	}
}

func TestGoldens_CoverEveryEventType(t *testing.T) {
	seen := map[EventType]bool{}
	for _, c := range goldenCases {
		for _, line := range streamLines(t, readGolden(t, c+".stream.ndjson")) {
			ev, err := DecodeEvent(line)
			require.NoError(t, err)
			seen[ev.EventType()] = true
		}
	}
	for _, typ := range EventTypes() {
		assert.True(t, seen[typ], "no golden emits %s", typ)
	}
	assert.Len(t, seen, len(EventTypes()))
}

func TestGoldens_SecretsAreTheFakeValues(t *testing.T) {
	for _, c := range goldenCases {
		raw := string(readGolden(t, c+".request.json"))
		var req TurnRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &req))
		assert.Equal(t, "sk-test", req.Agent.Model.APIKey, c)
		for _, s := range req.MCPServers {
			if s.Auth.Value != "" {
				assert.Equal(t, "tok-test", s.Auth.Value, c)
			}
		}
		assert.False(t, strings.Contains(string(readGolden(t, c+".stream.ndjson")), "sk-test"), "%s: the stream never echoes a secret", c)
	}
}

func TestGoldens_OnlyAnthropicNamesAProvider(t *testing.T) {
	for _, c := range goldenCases {
		req, err := DecodeRequest(readGolden(t, c+".request.json"))
		require.NoError(t, err, c)
		if c == "anthropic_reply" {
			assert.Equal(t, ProviderAnthropic, req.Agent.Model.Provider)
			continue
		}
		assert.Empty(t, req.Agent.Model.Provider, "%s: an OpenAI-compatible golden omits provider", c)
	}
}

func TestDecode_RefusesUnknownFieldsAndTypes(t *testing.T) {
	_, err := DecodeRequest([]byte(`{"contractVersion":1,"surprise":true}`))
	assert.Error(t, err)
	_, err = DecodeEvent([]byte(`{"type":"RUN_STARTED","threadId":"t","runId":"r","surprise":1}`))
	assert.Error(t, err)
	_, err = DecodeEvent([]byte(`{"type":"STATE_DELTA","delta":[]}`))
	assert.Error(t, err, "an event outside the contract")
	_, err = DecodeEvent([]byte(`not json`))
	assert.Error(t, err)
}

func assertToolResult(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var r struct {
		Status  string            `json:"status"`
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, strict(raw, &r), "a tool result is the Strands {status, content}: %s", raw)
	assert.Contains(t, []string{"success", "error"}, r.Status)
	require.NotEmpty(t, r.Content)
	for _, block := range r.Content {
		var b map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(block, &b))
		assert.Len(t, b, 1, "a content block is {text} or {json}: %s", block)
		_, text := b["text"]
		_, js := b["json"]
		assert.True(t, text || js, "%s", block)
	}
}

func assertAGUIMessage(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var m struct {
		ID         string            `json:"id"`
		Role       string            `json:"role"`
		Content    json.RawMessage   `json:"content,omitempty"`
		ToolCalls  []json.RawMessage `json:"toolCalls,omitempty"`
		ToolCallID string            `json:"toolCallId,omitempty"`
	}
	require.NoError(t, strict(raw, &m), "an AG-UI message: %s", raw)
	require.NotEmpty(t, m.ID)
	switch m.Role {
	case "user":
		assert.NotEmpty(t, m.Content)
	case "assistant":
		assert.True(t, len(m.Content) > 0 || len(m.ToolCalls) > 0, "%s", raw)
	case "tool":
		assert.NotEmpty(t, m.ToolCallID)
		assert.NotEmpty(t, m.Content)
	default:
		t.Errorf("unexpected role %q", m.Role)
	}
}

func TestGoldens_InterruptReasonIsJSON(t *testing.T) {
	ev, err := DecodeEvent([]byte(`{"type":"OAGENT_TERMINAL","status":"awaiting_approval","stopReason":"interrupt","snapshot":{},` +
		`"usage":{"inputTokens":1,"outputTokens":1,"cacheReadInputTokens":0,"cacheWriteInputTokens":0,"estimated":false},"messagesAppended":[],` +
		`"interrupts":[{"id":"hook:beforeToolCall:call_1:oagent:approval","name":"oagent:approval","reason":{"tool":"yasaku__record_expense","input":{"amount":1}}}],"notes":[]}`))
	require.NoError(t, err)
	term := ev.(*Terminal)
	require.Len(t, term.Interrupts, 1)
	assert.JSONEq(t, `{"tool":"yasaku__record_expense","input":{"amount":1}}`, string(term.Interrupts[0].Reason))
}

func TestGoldens_ResumeAndStoredResultUseTheInterrupt(t *testing.T) {
	for _, c := range []string{"resume", "stored_result"} {
		req, err := DecodeRequest(readGolden(t, c+".request.json"))
		require.NoError(t, err)
		assert.Equal(t, "resume", req.Input.Kind, c)
		require.Len(t, req.Input.Responses, 1, c)
		assert.Equal(t, "hook:beforeToolCall:call_1:oagent:approval", req.Input.Responses[0].InterruptID, c)
		assert.NotEqual(t, "null", string(req.Snapshot), c)
	}
	req, err := DecodeRequest(readGolden(t, "stored_result.request.json"))
	require.NoError(t, err)
	require.Len(t, req.StoredResults, 1)
	assert.Equal(t, "call_1", req.StoredResults[0].ToolUseID)
}
