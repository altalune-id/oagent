//go:build integration

package runtimetest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// FakeMCP is a streamable HTTP MCP server with one tool that counts its calls.
type FakeMCP struct {
	srv   *httptest.Server
	tool  string
	calls atomic.Int32
}

// NewFakeMCP starts the server on loopback with the one tool named tool.
func NewFakeMCP(t *testing.T, tool string) *FakeMCP {
	t.Helper()
	m := &FakeMCP{tool: tool}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("runtimetest: fake MCP listen: %v", err)
	}
	m.srv = httptest.NewUnstartedServer(http.HandlerFunc(m.serve))
	m.srv.Listener = ln
	m.srv.Start()
	t.Cleanup(m.srv.Close)
	return m
}

// Port is the server's TCP port.
func (m *FakeMCP) Port() int { return m.srv.Listener.Addr().(*net.TCPAddr).Port }

// Calls counts the tools/call requests the tool answered.
func (m *FakeMCP) Calls() int { return int(m.calls.Load()) }

func (m *FakeMCP) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(msg.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": msg.Params.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "fake", "version": "1"},
		}
	case "tools/list":
		result = map[string]any{"tools": []any{map[string]any{
			"name": m.tool, "description": "Record one entry.", "inputSchema": map[string]any{"type": "object"},
		}}}
	case "tools/call":
		n := m.calls.Add(1)
		result = map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf(`{"id":"entry_%d"}`, n)}}}
	default:
		w.Header().Set("Content-Type", "application/json")
		notFound := map[string]any{"code": -32601, "message": "not found"}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": notFound})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
}
