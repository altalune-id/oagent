package dataplane_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	clientThread = "client-thread-1"
	runPublicID  = "Rn5Q8wYk2HhT3mVz7XcLp"
)

var sseCases = []string{"plain_reply", "tool_call", "heartbeat_gap", "raw_dropped", "cancelled", "interrupted", "replay_settled"}

var relayed = []runtime.EventType{
	runtime.EventRunStarted, runtime.EventTextMessageStart, runtime.EventTextMessageContent, runtime.EventTextMessageEnd,
	runtime.EventToolCallStart, runtime.EventToolCallArgs, runtime.EventToolCallEnd, runtime.EventToolCallResult,
	runtime.EventRunFinished, runtime.EventRunError,
}

type sseEvent struct {
	id      int
	data    string
	comment string
}

func parseSSE(t *testing.T, raw []byte) []sseEvent {
	t.Helper()
	require.True(t, bytes.HasSuffix(raw, []byte("\n\n")), "the stream ends with a complete event")
	var out []sseEvent
	for block := range strings.SplitSeq(strings.TrimSuffix(string(raw), "\n\n"), "\n\n") {
		var e sseEvent
		for line := range strings.SplitSeq(block, "\n") {
			switch {
			case strings.HasPrefix(line, ": "):
				e.comment = strings.TrimPrefix(line, ": ")
			case strings.HasPrefix(line, "id: "):
				n, err := strconv.Atoi(strings.TrimPrefix(line, "id: "))
				require.NoError(t, err, line)
				e.id = n
			case strings.HasPrefix(line, "data: "):
				require.Empty(t, e.data, "one data line per event")
				e.data = strings.TrimPrefix(line, "data: ")
			default:
				t.Fatalf("not an SSE line we send: %q", line)
			}
		}
		out = append(out, e)
	}
	return out
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "sse", name))
	require.NoError(t, err)
	return raw
}

func TestGoldens_SSECasesWellFormed(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "sse", "*"))
	require.NoError(t, err)
	assert.Len(t, files, 2*len(sseCases), "an input and an output per case, nothing else")
	sessionID := regexp.MustCompile(`"t[0-9a-f]{32}"`)
	uuid := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	for _, c := range sseCases {
		t.Run(c, func(t *testing.T) {
			raw := readGolden(t, c+".sse")
			assert.False(t, sessionID.Match(raw), "no runtime session id leaves")
			assert.False(t, uuid.Match(raw), "no UUID leaves")
			assert.NotContains(t, string(raw), "event:", "data lines only, as AG-UI's encoder writes")
			next := 1
			var types []runtime.EventType
			for _, e := range parseSSE(t, raw) {
				if e.comment != "" {
					assert.Equal(t, "ping", e.comment)
					assert.Zero(t, e.id, "a heartbeat carries no id")
					assert.Empty(t, e.data)
					continue
				}
				assert.Equal(t, next, e.id, "ids count the run's relayed events from 1")
				next++
				ev, err := runtime.DecodeEvent([]byte(e.data))
				require.NoError(t, err, "data is a valid AG-UI event: %s", e.data)
				assert.Contains(t, relayed, ev.EventType(), "only allow-listed types are relayed")
				types = append(types, ev.EventType())
				var ids struct {
					ThreadID *string `json:"threadId"`
					RunID    *string `json:"runId"`
				}
				require.NoError(t, json.Unmarshal([]byte(e.data), &ids))
				if ids.ThreadID != nil {
					assert.Equal(t, clientThread, *ids.ThreadID, "threadId is the client's")
				}
				if ids.RunID != nil {
					assert.Equal(t, runPublicID, *ids.RunID, "runId is the run public id")
				}
			}
			require.NotEmpty(t, types)
			assert.Equal(t, runtime.EventRunStarted, types[0])
			last := types[len(types)-1]
			assert.True(t, last == runtime.EventRunFinished || last == runtime.EventRunError, "the stream ends the run")
			assert.Equal(t, 1, strings.Count(string(raw), `"type":"RUN_STARTED"`))
		})
	}
}

func TestGoldens_InputsAreWhatTheRelayReads(t *testing.T) {
	for _, c := range sseCases {
		t.Run(c, func(t *testing.T) {
			sc := bufio.NewScanner(bytes.NewReader(readGolden(t, c+".ndjson")))
			sc.Buffer(nil, 1<<20)
			lines := 0
			for sc.Scan() {
				lines++
				var head struct {
					Type string `json:"type"`
					Role string `json:"role"`
				}
				require.NoError(t, json.Unmarshal(sc.Bytes(), &head), sc.Text())
				if c == "replay_settled" {
					assert.NotEmpty(t, head.Role, "a replay is built from stored AG-UI messages")
					continue
				}
				assert.NotEmpty(t, head.Type)
				if head.Type == "RAW" {
					continue
				}
				_, err := runtime.DecodeEvent(sc.Bytes())
				assert.NoError(t, err, "every other line is a runtime contract event")
			}
			assert.Positive(t, lines)
		})
	}
}

func TestGoldens_DropRules(t *testing.T) {
	for _, c := range sseCases {
		out := string(readGolden(t, c+".sse"))
		for _, never := range []string{"OAGENT_CHECKPOINT", "OAGENT_TERMINAL", "OAGENT_HEARTBEAT", `"RAW"`} {
			assert.NotContains(t, out, never, "%s: %s is never relayed", c, never)
		}
	}
	in := string(readGolden(t, "raw_dropped.ndjson"))
	assert.Contains(t, in, `"type":"RAW"`)
	heartbeats := strings.Count(string(readGolden(t, "heartbeat_gap.ndjson")), "OAGENT_HEARTBEAT")
	assert.Equal(t, heartbeats, strings.Count(string(readGolden(t, "heartbeat_gap.sse")), ": ping\n\n"), "each runtime heartbeat is one ping")
	for c, code := range map[string]string{"cancelled": "cancelled", "interrupted": "interrupted"} {
		out := string(readGolden(t, c+".sse"))
		assert.Contains(t, out, `"type":"RUN_ERROR"`)
		assert.Contains(t, out, `"code":"`+code+`"`)
		assert.NotContains(t, string(readGolden(t, c+".ndjson")), "OAGENT_TERMINAL", "%s: Go cut the turn; no terminal came", c)
	}
}
