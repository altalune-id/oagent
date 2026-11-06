//go:build integration

package runtimetest

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/runtime"
	runtimehttp "altalune.id/oagent/internal/kernel/runtime/http"
)

type fakeThread struct {
	pending     json.RawMessage
	checkpoints []*runtime.Checkpoint
	runID       uuid.UUID
	status      string
	attempt     int
	roles       []string
	snapshot    json.RawMessage
	usage       Usage
}

func (th *fakeThread) checkpointed(callID string) bool {
	for _, c := range th.checkpoints {
		if c.ToolUseID == callID {
			return true
		}
	}
	return false
}

type fakeDriver struct {
	env     Env
	client  *runtimehttp.Client
	mu      sync.Mutex
	threads map[uuid.UUID]*fakeThread
}

func newFakeDriver(t *testing.T, env Env) Driver {
	t.Helper()
	c, err := runtimehttp.New(runtime.Config{Driver: runtime.DriverHTTP, URL: env.RuntimeURL, Token: env.RuntimeToken})
	if err != nil {
		t.Fatalf("fake driver: %v", err)
	}
	return &fakeDriver{env: env, client: c, threads: map[uuid.UUID]*fakeThread{}}
}

func (d *fakeDriver) Seed(_ context.Context, _ Env, text string) (Seeded, error) {
	msg, err := json.Marshal(map[string]string{"id": "msg_user_1", "role": "user", "content": text})
	if err != nil {
		return Seeded{}, err
	}
	id := uuid.New()
	d.mu.Lock()
	d.threads[id] = &fakeThread{pending: msg, roles: []string{"user"}}
	d.mu.Unlock()
	return Seeded{ThreadID: id}, nil
}

func (d *fakeDriver) request(th *fakeThread, threadID uuid.UUID) runtime.TurnRequest {
	maxTokens := int32(256)
	req := runtime.TurnRequest{
		ContractVersion: runtime.ContractVersion,
		IDs: runtime.TurnIDs{
			Run: th.runID.String(), Thread: "t" + threadID.String()[:8], Agent: "aharness",
			AgentVersion: 1, Attempt: int32(th.attempt), Project: "pharness",
		},
		Agent: runtime.AgentSpec{
			Instructions: "Be brief.", Temperature: 0, MaxSteps: 4,
			Model: runtime.ModelSpec{BaseURL: d.env.ModelBaseURL, APIKey: d.env.ModelKey, ModelID: d.env.ModelID, MaxTokens: &maxTokens},
		},
		MCPServers:    []runtime.MCPServerSpec{},
		Snapshot:      json.RawMessage("null"),
		History:       []json.RawMessage{},
		Input:         runtime.TurnInput{Kind: "message", Message: th.pending},
		StoredResults: []runtime.StoredResult{},
		Limits:        runtime.Limits{DeadlineMs: 60_000},
	}
	if d.env.MCPURL != "" {
		req.MCPServers = []runtime.MCPServerSpec{{
			Name: d.env.MCPServerName, URL: d.env.MCPURL, Auth: runtime.MCPAuth{Kind: "none"},
			AllowedTools: []string{d.env.MCPToolName}, ConnectTimeoutMs: 5000, CallTimeoutMs: 5000,
		}}
	}
	if th.attempt > 1 && len(th.checkpoints) > 0 {
		req.History = append([]json.RawMessage{th.pending}, toolRounds(th.checkpoints)...)
		req.Input = runtime.TurnInput{Kind: "continue"}
		for _, c := range th.checkpoints {
			req.StoredResults = append(req.StoredResults, runtime.StoredResult{ToolUseID: c.ToolUseID, Result: c.Result})
		}
	}
	return req
}

func toolRounds(cps []*runtime.Checkpoint) []json.RawMessage {
	out := make([]json.RawMessage, 0, 2*len(cps))
	for _, c := range cps {
		call, _ := json.Marshal(map[string]any{"id": "msg_asst_" + c.ToolUseID, "role": "assistant", "toolCalls": []any{map[string]any{
			"id": c.ToolUseID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": string(c.Input)},
		}}})
		var res struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		_ = json.Unmarshal(c.Result, &res)
		text := ""
		for _, b := range res.Content {
			text += b.Text
		}
		result, _ := json.Marshal(map[string]any{"id": "msg_tool_" + c.ToolUseID, "role": "tool", "toolCallId": c.ToolUseID, "content": text})
		out = append(out, call, result)
	}
	return out
}

func (d *fakeDriver) Execute(ctx context.Context, threadID uuid.UUID, _ string) (Outcome, error) {
	d.mu.Lock()
	th := d.threads[threadID]
	if th.runID == uuid.Nil {
		th.runID = uuid.New()
	}
	th.attempt++
	th.status = "running"
	req := d.request(th, threadID)
	d.mu.Unlock()

	interrupted := func(err error) (Outcome, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		th.status = "interrupted"
		return Outcome{RunID: th.runID, Status: th.status, Attempt: th.attempt}, err
	}
	stream, err := d.client.Turn(ctx, req)
	if err != nil {
		return interrupted(err)
	}
	defer func() { _ = stream.Close() }()
	for {
		ev, err := stream.Next()
		if err != nil {
			return interrupted(err)
		}
		if cp, ok := ev.(*runtime.Checkpoint); ok {
			d.mu.Lock()
			th.checkpoints = append(th.checkpoints, cp)
			th.roles = append(th.roles, "assistant", "tool")
			d.mu.Unlock()
			continue
		}
		term, ok := ev.(*runtime.Terminal)
		if !ok {
			continue
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		th.status = term.Status
		th.snapshot = term.Snapshot
		th.usage = th.usage.plus(Usage{
			InputTokens: term.Usage.InputTokens, OutputTokens: term.Usage.OutputTokens,
			CacheReadInputTokens: term.Usage.CacheReadInputTokens, CacheWriteInputTokens: term.Usage.CacheWriteInputTokens,
		})
		for _, m := range term.MessagesAppended {
			var head struct {
				Role       string `json:"role"`
				ToolCallID string `json:"toolCallId"`
				ToolCalls  []struct {
					ID string `json:"id"`
				} `json:"toolCalls"`
			}
			if err := json.Unmarshal(m, &head); err != nil {
				return Outcome{}, err
			}
			callID := head.ToolCallID
			if len(head.ToolCalls) > 0 {
				callID = head.ToolCalls[0].ID
			}
			if callID != "" && th.checkpointed(callID) {
				continue
			}
			if head.Role == "assistant" || head.Role == "tool" {
				th.roles = append(th.roles, head.Role)
			}
		}
		if term.Status == "" {
			return Outcome{}, errors.New("fake driver: terminal without status")
		}
		return Outcome{RunID: th.runID, Status: th.status, Attempt: th.attempt}, nil
	}
}

func (d *fakeDriver) State(_ context.Context, threadID uuid.UUID) (State, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	th := d.threads[threadID]
	saved := len(th.snapshot) > 0 && string(th.snapshot) != "null"
	return State{
		RunID: th.runID, RunStatus: th.status, Attempt: th.attempt,
		Roles: append([]string(nil), th.roles...), SnapshotSaved: saved, Usage: th.usage,
	}, nil
}

func TestHarness_FakeDriver(t *testing.T) {
	Run(t, newFakeDriver)
}
