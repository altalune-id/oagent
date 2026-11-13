package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var goldenCases = []string{
	"plain_reply", "tool_call", "mcp_server_down", "resume", "rebuild_from_history", "stored_result", "limit_reached", "cancel", "model_error", "resume_after_crash",
	"image_input", "image_refused", "anthropic_reply",
	"approval_interrupt", "approval_rule_broken", "approval_resume_approved", "approval_resume_rejected", "approval_hash_mismatch",
	"approval_absent_only",
	"knowledge_injected", "knowledge_absent", "knowledge_empty", "knowledge_hostile",
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
			assert.Equal(t, ContractVersion, req.ContractVersion)
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

func TestGoldens_KnowledgeOnlyInTheRequest(t *testing.T) {
	req, err := DecodeRequest(readGolden(t, "knowledge_injected.request.json"))
	require.NoError(t, err)
	require.Len(t, req.Knowledge, 2)
	assert.Equal(t, Passage{Text: "Refunds are paid back to the original card within 7 days.", Title: "Refund policy", Heading: "Timing", Page: new(int32(2))}, req.Knowledge[0])
	stream := string(readGolden(t, "knowledge_injected.stream.ndjson"))
	for _, p := range req.Knowledge {
		assert.NotContains(t, stream, p.Text, "passage text never reaches the stream")
	}

	absent, err := DecodeRequest(readGolden(t, "knowledge_absent.request.json"))
	require.NoError(t, err)
	assert.Nil(t, absent.Knowledge)
	empty, err := DecodeRequest(readGolden(t, "knowledge_empty.request.json"))
	require.NoError(t, err)
	assert.Empty(t, empty.Knowledge)
	assert.NotNil(t, empty.Knowledge)
}

func TestTurnRequest_KnowledgeOmittedWhenEmpty(t *testing.T) {
	for _, k := range [][]Passage{nil, {}} {
		raw, err := json.Marshal(TurnRequest{Knowledge: k})
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "knowledge")
	}
	raw, err := json.Marshal(TurnRequest{Knowledge: []Passage{{Text: "t", Title: "T"}}})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"knowledge":[{"text":"t","title":"T"}]`)
}

func TestPassageBounds_MatchTheRuntime(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "apps", "runtime", "src", "contract", "request.ts"))
	require.NoError(t, err)
	src := string(raw)
	for _, want := range []string{
		fmt.Sprintf("MAX_PASSAGES = %d;", MaxPassages),
		fmt.Sprintf("MAX_PASSAGE_CHARS = %d_%03d;", MaxPassageTextBytes/1000, MaxPassageTextBytes%1000),
		"text: z.string().min(1).max(MAX_PASSAGE_CHARS)",
		fmt.Sprintf("title: z.string().min(1).max(%d)", MaxPassageLabelBytes),
		"page: z.number().int().positive().optional()",
		fmt.Sprintf("heading: z.string().max(%d)", MaxPassageLabelBytes),
		fmt.Sprintf("url: z.string().max(%d)", MaxPassageURLBytes),
	} {
		assert.Contains(t, src, want)
	}
}

func TestNormalizePassages(t *testing.T) {
	long := strings.Repeat("€", 3000)
	in := []Passage{
		{Text: long, Title: strings.Repeat("€", MaxPassageLabelBytes), Heading: "h", Page: new(int32(3)), URL: "https://a.example.com/x"},
		{Text: "  ", Title: "dropped: no text"},
		{Text: "no title", Title: " ", Page: new(int32(0)), URL: "https://a.example.com/" + strings.Repeat("u", MaxPassageURLBytes)},
		{Text: "bad \xff utf8", Title: "T"},
		{Text: "4", Title: "T"}, {Text: "5", Title: "T"}, {Text: "6", Title: "T"},
	}
	out := NormalizePassages(in, MaxKnowledgeTextBytes)

	require.Len(t, out, 6, "every passage with text, up to MaxPassages")
	first := out[0]
	assert.Equal(t, long, first.Text)
	assert.True(t, utf8.ValidString(first.Title))
	assert.LessOrEqual(t, len(first.Title), MaxPassageLabelBytes)
	assert.Equal(t, "https://a.example.com/x", first.URL)
	assert.Equal(t, int32(3), *first.Page)

	third := out[1]
	assert.Equal(t, "no title", third.Text)
	assert.Equal(t, UntitledPassage, third.Title)
	assert.Nil(t, third.Page, "page 0 is not sent")
	assert.Empty(t, third.URL, "a URL over the cap is dropped, never cut")

	assert.Equal(t, "bad \uFFFD utf8", out[2].Text)
	assert.Equal(t, []string{"4", "5", "6"}, []string{out[3].Text, out[4].Text, out[5].Text})
	assert.Equal(t, "€", in[0].Text[:3], "the input is not modified")
	assert.Len(t, in[0].Text, len(long))
	assert.Empty(t, NormalizePassages(nil, MaxKnowledgeTextBytes))
}

func TestGoldens_KnowledgeHostileSentAsIs(t *testing.T) {
	req, err := DecodeRequest(readGolden(t, "knowledge_hostile.request.json"))
	require.NoError(t, err)
	require.Len(t, req.Knowledge, 1)
	p := req.Knowledge[0]
	for _, probe := range []string{"<\u200b/passage>", "\uff1c\uff0f\uff50", "\ufe64/passage\ufe65", "\u3008/passage\u3009", "</pa\u017f\u017fage>", "</tool_output>"} {
		assert.Contains(t, p.Text, probe, "the golden carries the look-alike %q", probe)
	}
	assert.Equal(t, req.Knowledge, NormalizePassages(req.Knowledge, MaxKnowledgeTextBytes), "Go sends hostile text as is; the runtime escapes it")
	stream := string(readGolden(t, "knowledge_hostile.stream.ndjson"))
	assert.NotContains(t, stream, "Refunds are instant")
	assert.NotContains(t, stream, "passage")
}

func TestNormalizePassages_FormatCharsAndHeadings(t *testing.T) {
	out := NormalizePassages([]Passage{
		{Text: "\u200b\u2060 \u00ad\n", Title: "only format chars"},
		{Text: "kept", Title: "\u200b\u2060", Heading: " \t\n"},
		{Text: "cut", Title: "T", Heading: strings.Repeat("\u20ac", MaxPassageLabelBytes)},
	}, MaxKnowledgeTextBytes)
	require.Len(t, out, 2)
	assert.Equal(t, UntitledPassage, out[0].Title, "a title of format characters only is untitled")
	assert.Empty(t, out[0].Heading, "a whitespace-only heading is left out")
	assert.LessOrEqual(t, len(out[1].Heading), MaxPassageLabelBytes)
	assert.Greater(t, len(out[1].Heading), MaxPassageLabelBytes-3)
	assert.True(t, utf8.ValidString(out[1].Heading))
}

func TestNormalizePassages_Budget(t *testing.T) {
	six := strings.Repeat("a", 6000)
	in := []Passage{{Text: six, Title: "1"}, {Text: six, Title: "2"}, {Text: six, Title: "3"}, {Text: "short", Title: "4"}}
	out := NormalizePassages(in, 16000)
	require.Len(t, out, 2, "the passage that would pass the budget and every later one are dropped")
	total := 0
	for _, p := range out {
		total += len(p.Text)
	}
	assert.LessOrEqual(t, total, 16000)
	assert.Len(t, NormalizePassages(in, 7000), 1, "a lower budget keeps fewer passages")
	assert.Len(t, NormalizePassages(in, MaxKnowledgeTextBytes), 4, "the largest budget keeps them all")
	assert.Len(t, NormalizePassages(in, 10*MaxKnowledgeTextBytes), 4)
	assert.Empty(t, NormalizePassages(in, 0), "no budget, no knowledge")

	big := []Passage{{Text: strings.Repeat("€", 4*MaxPassageTextBytes), Title: "big"}}
	one := NormalizePassages(big, 10*MaxKnowledgeTextBytes)
	require.Len(t, one, 1, "a first passage over the budget is cut, not dropped")
	assert.LessOrEqual(t, len(one[0].Text), MaxPassageTextBytes, "a budget above the cap never lifts the per-passage cap")
	cut := NormalizePassages([]Passage{{Text: strings.Repeat("€", MaxPassageTextBytes), Title: "big"}}, 1000)
	require.Len(t, cut, 1)
	assert.LessOrEqual(t, len(cut[0].Text)+len(cut[0].Title), 1000)
	assert.Greater(t, len(cut[0].Text)+len(cut[0].Title), 1000-3)
}

func TestNormalizePassages_TotalCapCountsLabels(t *testing.T) {
	label := strings.Repeat("t", MaxPassageLabelBytes)
	url := "https://x.example.com/" + strings.Repeat("u", MaxPassageURLBytes-30)
	in := make([]Passage, MaxPassages)
	for i := range in {
		in[i] = Passage{Text: strings.Repeat("a", 2000), Title: label, Heading: label, URL: url}
	}
	out := NormalizePassages(in, 16000)
	total := 0
	for _, p := range out {
		total += len(p.Text) + len(p.Title) + len(p.Heading) + len(p.URL)
	}
	assert.LessOrEqual(t, total, 16000, "titles, headings and urls count toward the budget")
	assert.Len(t, out, 3)
}

func TestPassageBounds_TenPassagesAndATokenBudgetOf8000(t *testing.T) {
	assert.Equal(t, 10, MaxPassages)
	assert.Equal(t, 8000*4, MaxKnowledgeTextBytes)
}
