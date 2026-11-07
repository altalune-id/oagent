//go:build integration

// Package runtimetest is the run acceptance harness: the runtime container, a fake model and the driver scenarios.
package runtimetest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// ModelID is the model the fake server answers for.
const ModelID = "fake-model"

// Reply is one scripted answer of the fake model; Hang blocks the call until the client goes away.
type Reply struct {
	Chunks       []string
	InputTokens  int64
	OutputTokens int64
	Hang         bool
	// ToolUnlessResult calls this tool with a fresh call id, unless the request already carries a tool result; then it answers Chunks.
	ToolUnlessResult string
}

// FakeModel is an OpenAI-compatible chat completions server that streams scripted replies.
type FakeModel struct {
	srv     *httptest.Server
	mu      sync.Mutex
	replies []Reply
	calls   int
	bodies  [][]byte
	hung    chan struct{}
}

// NewFakeModel starts the server on loopback; containers reach it through the testcontainers host tunnel.
func NewFakeModel(t *testing.T, replies ...Reply) *FakeModel {
	t.Helper()
	m := &FakeModel{replies: replies, hung: make(chan struct{}, 8)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("runtimetest: fake model listen: %v", err)
	}
	m.srv = httptest.NewUnstartedServer(http.HandlerFunc(m.serve))
	m.srv.Listener = ln
	m.srv.Start()
	t.Cleanup(m.srv.Close)
	return m
}

// Port is the server's TCP port.
func (m *FakeModel) Port() int { return m.srv.Listener.Addr().(*net.TCPAddr).Port }

// Calls counts the chat completion requests served so far.
func (m *FakeModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// Requests returns the raw body of every chat completion request served so far, in order.
func (m *FakeModel) Requests() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]byte(nil), m.bodies...)
}

// Hung receives once per reply that started hanging.
func (m *FakeModel) Hung() <-chan struct{} { return m.hung }

func (m *FakeModel) next() (Reply, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if len(m.replies) == 0 {
		return Reply{}, false
	}
	r := m.replies[0]
	m.replies = m.replies[1:]
	return r, true
}

func chunk(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func delta(d map[string]any, finish any) map[string]any {
	return map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 0, "model": ModelID,
		"choices": []any{map[string]any{"index": 0, "delta": d, "finish_reason": finish}},
	}
}

func (m *FakeModel) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	raw, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.bodies = append(m.bodies, raw)
	m.mu.Unlock()
	_ = json.Unmarshal(raw, &req)
	reply, ok := m.next()
	if !ok {
		http.Error(w, `{"error":{"message":"no scripted reply"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	chunk(w, delta(map[string]any{"role": "assistant", "content": ""}, nil))
	if reply.Hang {
		m.hung <- struct{}{}
		<-r.Context().Done()
		return
	}
	sawResult := false
	for _, msg := range req.Messages {
		sawResult = sawResult || msg.Role == "tool"
	}
	finish := "stop"
	if reply.ToolUnlessResult != "" && !sawResult {
		id := "call_" + strconv.Itoa(m.Calls()) + "_" + strconv.FormatInt(int64(len(req.Messages)), 10)
		chunk(w, delta(map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": id, "type": "function", "function": map[string]any{"name": reply.ToolUnlessResult, "arguments": "{}"},
		}}}, nil))
		finish = "tool_calls"
	} else {
		for _, c := range reply.Chunks {
			chunk(w, delta(map[string]any{"content": c}, nil))
		}
	}
	chunk(w, delta(map[string]any{}, finish))
	chunk(w, map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 0, "model": ModelID, "choices": []any{},
		"usage": map[string]any{
			"prompt_tokens": reply.InputTokens, "completion_tokens": reply.OutputTokens,
			"total_tokens": reply.InputTokens + reply.OutputTokens,
		},
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}
