package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/runtime"
)

func (s *Service) request(ctx context.Context, t *turn) (runtime.TurnRequest, []json.RawMessage, error) {
	r, v := t.run, t.version
	req := runtime.TurnRequest{
		ContractVersion: runtime.ContractVersion,
		IDs: runtime.TurnIDs{
			Run: r.PublicID, Thread: SessionID(r.ThreadID), Agent: AgentKey(v.AgentID),
			AgentVersion: v.Version, Attempt: r.Attempt, Project: r.ProjectID.String(),
		},
		Agent: runtime.AgentSpec{
			Instructions: v.Instructions, Temperature: v.Temperature, MaxSteps: v.MaxSteps,
			Model: runtime.ModelSpec{BaseURL: t.model.BaseURL, APIKey: t.model.APIKey, ModelID: v.Model},
		},
		MCPServers:    []runtime.MCPServerSpec{},
		History:       []json.RawMessage{},
		StoredResults: []runtime.StoredResult{},
		Limits:        runtime.Limits{DeadlineMs: (s.turnTimeout - deadlineMargin).Milliseconds(), Budget: s.budget(t)},
	}
	servers, err := s.mcpServers(ctx, v)
	if err != nil {
		return req, nil, err
	}
	req.MCPServers = servers
	if req.Input, err = input(t.taken); err != nil {
		return req, nil, err
	}
	stored, err := s.store.StoredResults(ctx, r.ID)
	if err != nil {
		return req, nil, err
	}
	for _, sr := range stored {
		t.stored[sr.ToolUseID] = true
		var body struct {
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(sr.Body, &body) == nil && len(body.Result) > 0 {
			req.StoredResults = append(req.StoredResults, runtime.StoredResult{ToolUseID: sr.ToolUseID, Result: body.Result})
		}
	}
	if r.Attempt > 1 && len(req.StoredResults) > 0 {
		all, err := s.store.History(ctx, r.ThreadID, uuid.Nil)
		if err != nil {
			return req, nil, err
		}
		req.Input, req.History = runtime.TurnInput{Kind: runtime.InputContinue}, rebuild(all)
		return req, req.History, nil
	}
	msgs, err := s.store.History(ctx, r.ThreadID, r.ID)
	if err != nil {
		return req, nil, err
	}
	history := rebuild(msgs)
	if snap := s.snapshot(ctx, t); snap != nil {
		req.Snapshot = snap.Blob
		return req, history, nil
	}
	req.History = history
	return req, history, nil
}

func (s *Service) snapshot(ctx context.Context, t *turn) *Snapshot {
	if s.snaps == nil {
		return nil
	}
	snap, err := s.snaps.Load(ctx, t.run.ThreadID)
	if err != nil {
		s.log.WarnContext(ctx, "run: snapshot load failed; rebuilding from messages", "err", err.Error())
		return nil
	}
	if snap == nil || snap.AgentVersionID != t.run.AgentVersionID {
		return nil
	}
	return snap
}

func (s *Service) budget(t *turn) *runtime.Budget {
	p := t.run.Prices
	if t.version.MaxCostCents <= 0 || p == nil {
		return nil
	}
	spent, _ := Cost(t.run.Usage, p)
	val := func(v *int64) int64 {
		if v == nil {
			return 0
		}
		return *v
	}
	return &runtime.Budget{
		RemainingMicros: max(int64(t.version.MaxCostCents)*10_000-spent, 0),
		Prices:          runtime.Prices{Input: val(p.Input), Output: val(p.Output), CacheRead: val(p.CacheRead), CacheWrite: val(p.CacheWrite)},
	}
}

func (s *Service) mcpServers(ctx context.Context, v *AgentVersion) ([]runtime.MCPServerSpec, error) {
	out := []runtime.MCPServerSpec{}
	for _, g := range v.MCPServers {
		var allowed []string
		for _, tool := range g.Tools {
			if !tool.RequireApproval {
				allowed = append(allowed, tool.Name)
			}
		}
		if len(allowed) == 0 {
			continue
		}
		srv, err := s.servers.Open(ctx, g.ServerID)
		if err != nil {
			return nil, fmt.Errorf("run: open mcp server %s: %w", g.ServerID, err)
		}
		out = append(out, runtime.MCPServerSpec{
			Name: srv.Name, URL: srv.URL, AllowedTools: allowed,
			Auth:             runtime.MCPAuth{Kind: srv.AuthKind, Header: srv.AuthHeader, Value: srv.AuthValue},
			ConnectTimeoutMs: connectTimeoutMs, CallTimeoutMs: srv.TimeoutMS,
		})
	}
	return out, nil
}

type agUIMessage struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolCalls  []struct {
		ID string `json:"id"`
	} `json:"toolCalls,omitempty"`
}

func input(taken []Message) (runtime.TurnInput, error) {
	var users []agUIMessage
	for _, m := range taken {
		if m.Role != RoleUser {
			continue
		}
		var u agUIMessage
		if err := json.Unmarshal(m.Body, &u); err != nil {
			return runtime.TurnInput{}, fmt.Errorf("run: user message %d: %w", m.Seq, err)
		}
		users = append(users, u)
	}
	if len(users) == 0 {
		return runtime.TurnInput{}, errors.New("run: the turn has no user message")
	}
	if len(users) == 1 {
		body, err := json.Marshal(map[string]json.RawMessage{"id": quote(users[0].ID), "role": quote("user"), "content": users[0].Content})
		return runtime.TurnInput{Kind: "message", Message: body}, err
	}
	parts := make([]string, 0, len(users))
	for _, u := range users {
		var text string
		if json.Unmarshal(u.Content, &text) != nil {
			text = string(u.Content)
		}
		parts = append(parts, text)
	}
	body, err := json.Marshal(map[string]string{"id": users[len(users)-1].ID, "role": "user", "content": strings.Join(parts, "\n\n")})
	return runtime.TurnInput{Kind: "message", Message: body}, err
}

func quote(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func rebuild(msgs []Message) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(msgs))
	answered := map[string]bool{}
	var asked []string
	for _, m := range msgs {
		var am agUIMessage
		if err := json.Unmarshal(m.Body, &am); err != nil {
			continue
		}
		if m.Role == RoleTool {
			answered[am.ToolCallID] = true
			body, _ := json.Marshal(map[string]json.RawMessage{
				"id": quote(am.ID), "role": quote("tool"), "toolCallId": quote(am.ToolCallID), "content": am.Content,
			})
			out = append(out, body)
			continue
		}
		for _, c := range am.ToolCalls {
			asked = append(asked, c.ID)
		}
		out = append(out, m.Body)
	}
	for _, id := range asked {
		if answered[id] {
			continue
		}
		body, _ := json.Marshal(map[string]string{"id": "expired_" + id, "role": "tool", "toolCallId": id, "content": expiredToolResult})
		out = append(out, body)
	}
	return out
}

type strandsResult struct {
	Status  string `json:"status"`
	Content []struct {
		Text *string         `json:"text"`
		JSON json.RawMessage `json:"json"`
	} `json:"content"`
}

func callMessage(cp *runtime.Checkpoint, start *runtime.ToolCallStart) Message {
	id, name := "call_"+cp.ToolUseID, cp.Name
	if start != nil && start.ParentMessageID != "" {
		id = start.ParentMessageID + ":" + cp.ToolUseID
	}
	args := string(cp.Input)
	if args == "" || args == "null" {
		args = "{}"
	}
	raw, _ := json.Marshal(map[string]any{
		"id": id, "role": "assistant",
		"toolCalls": []map[string]any{{"id": cp.ToolUseID, "type": "function", "function": map[string]string{"name": name, "arguments": args}}},
	})
	return Message{Role: RoleAssistant, Body: raw}
}

func checkpointMessage(cp *runtime.Checkpoint) Message {
	var res strandsResult
	_ = json.Unmarshal(cp.Result, &res)
	parts := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		switch {
		case c.Text != nil:
			parts = append(parts, *c.Text)
		case len(c.JSON) > 0:
			parts = append(parts, string(c.JSON))
		}
	}
	body := map[string]any{
		"id": "tool_" + cp.ToolUseID, "role": "tool", "toolCallId": cp.ToolUseID,
		"content": strings.Join(parts, "\n"), "result": cp.Result,
	}
	if res.Status == "error" {
		body["error"] = strings.Join(parts, "\n")
	}
	raw, _ := json.Marshal(body)
	return Message{Role: RoleTool, Body: raw, ToolCallID: cp.ToolUseID}
}

func (s *Service) stream(ctx context.Context, t *turn, req runtime.TurnRequest, history []json.RawMessage) (*Run, error) {
	st, err := s.rt.Turn(ctx, req)
	if runtime.IsSnapshotUnloadableError(err) && req.Snapshot != nil {
		req.Snapshot, req.History = nil, history
		st, err = s.rt.Turn(ctx, req)
	}
	if err != nil {
		return s.refused(ctx, t, err)
	}
	defer func() { _ = st.Close() }()
	est := estimate{input: int64(len(mustJSON(req)))}
	calls := map[string]*runtime.ToolCallStart{}
	for {
		e, err := st.Next()
		if err != nil {
			return s.cut(ctx, t, est, err)
		}
		switch ev := e.(type) {
		case *runtime.TextMessageContent:
			est.output += int64(len(ev.Delta))
		case *runtime.ToolCallArgs:
			est.output += int64(len(ev.Delta))
		case *runtime.ToolCallStart:
			calls[ev.ToolCallID] = ev
		case *runtime.Checkpoint:
			msgs := []Message{callMessage(ev, calls[ev.ToolUseID]), checkpointMessage(ev)}
			if err := s.store.PutCheckpoint(context.WithoutCancel(ctx), t.token, t.run.ID, ev.ToolUseID, msgs); err != nil {
				return s.abandon(ctx, t, add(t.run.Usage, est.usage()), err)
			}
			t.stored[ev.ToolUseID] = true
		case *runtime.Terminal:
			return s.finish(ctx, t, ev)
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

type estimate struct{ input, output int64 }

func (e estimate) usage() Usage {
	tokens := func(chars int64) int64 { return (chars + charsPerToken - 1) / charsPerToken }
	return Usage{InputTokens: tokens(e.input), OutputTokens: tokens(e.output), Estimated: true}
}

func (s *Service) refused(ctx context.Context, t *turn, err error) (*Run, error) {
	if runtime.IsContractError(err) || (runtime.IsStatusError(err) && !transientStatus(err)) || runtime.IsSnapshotUnloadableError(err) {
		s.log.ErrorContext(ctx, "run: runtime refused the turn", "run", t.run.PublicID, "err", err.Error())
		return s.fail(ctx, t, apperror.CodeRunRuntimeFailed)
	}
	s.log.WarnContext(ctx, "run: runtime unavailable; the run waits for a retry", "run", t.run.PublicID, "err", err.Error())
	if err := t.run.Move(StatusInterrupted, time.Now()); err != nil {
		return nil, s.errOrReport(ctx, "run: interrupt", err)
	}
	t.run.Attempt = max(t.run.Attempt-1, 1)
	return s.settle(ctx, t, t.run.Usage, nil, nil)
}

func (s *Service) cut(ctx context.Context, t *turn, est estimate, cause error) (*Run, error) {
	to := StatusInterrupted
	if errors.Is(context.Cause(ctx), ErrCancelRequested) {
		to = StatusCancelled
	}
	if ctx.Err() == nil {
		s.log.WarnContext(ctx, "run: turn stream cut", "run", t.run.PublicID, "err", cause.Error())
	}
	if err := t.run.Move(to, time.Now()); err != nil {
		return nil, s.errOrReport(ctx, "run: cut", err)
	}
	return s.settle(ctx, t, add(t.run.Usage, est.usage()), nil, nil)
}

func add(a, b Usage) Usage {
	return Usage{
		InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens,
		CacheReadTokens: a.CacheReadTokens + b.CacheReadTokens, CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
		Estimated: a.Estimated || b.Estimated,
	}
}

func (s *Service) finish(ctx context.Context, t *turn, term *runtime.Terminal) (*Run, error) {
	to, code := terminalStatus(term)
	if err := t.run.Move(to, time.Now()); err != nil {
		return nil, s.errOrReport(ctx, "run: finish", err)
	}
	t.run.ErrorCode = string(code)
	u := add(t.run.Usage, Usage{
		InputTokens:     max(term.Usage.InputTokens-term.Usage.CacheReadInputTokens-term.Usage.CacheWriteInputTokens, 0),
		OutputTokens:    term.Usage.OutputTokens,
		CacheReadTokens: term.Usage.CacheReadInputTokens, CacheWriteTokens: term.Usage.CacheWriteInputTokens,
		Estimated: term.Usage.Estimated,
	})
	return s.settle(ctx, t, u, finalMessages(term.MessagesAppended, t.stored), s.terminalSnapshot(t, term.Snapshot))
}

func terminalStatus(term *runtime.Terminal) (Status, apperror.Code) {
	switch term.Status {
	case runtime.StatusCompleted:
		return StatusCompleted, ""
	case runtime.StatusAwaitingApproval:
		return StatusAwaitingApproval, ""
	case runtime.StatusLimitReached:
		return StatusLimitReached, ""
	case runtime.StatusCancelled:
		if term.Error != nil && term.Error.Code == "deadline" {
			return StatusFailed, apperror.CodeRunTimedOut
		}
		return StatusCancelled, ""
	default:
		return StatusFailed, apperror.CodeRunRuntimeFailed
	}
}

func finalMessages(appended []json.RawMessage, stored map[string]bool) []Message {
	var out []Message
	for _, raw := range appended {
		var am agUIMessage
		if json.Unmarshal(raw, &am) != nil || am.Role == string(RoleUser) {
			continue
		}
		if am.Role == string(RoleTool) {
			if stored[am.ToolCallID] {
				continue
			}
			out = append(out, Message{Role: RoleTool, Body: raw, ToolCallID: am.ToolCallID})
			continue
		}
		if len(am.ToolCalls) > 0 && allStored(am, stored) {
			if !hasText(am.Content) {
				continue
			}
			raw, _ = json.Marshal(map[string]json.RawMessage{"id": quote(am.ID), "role": quote(am.Role), "content": am.Content})
		}
		out = append(out, Message{Role: Role(am.Role), Body: raw})
	}
	return out
}

func allStored(am agUIMessage, stored map[string]bool) bool {
	for _, c := range am.ToolCalls {
		if !stored[c.ID] {
			return false
		}
	}
	return true
}

func hasText(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return strings.TrimSpace(text) != ""
	}
	return len(content) > 0 && string(content) != "null"
}

func (s *Service) terminalSnapshot(t *turn, raw json.RawMessage) *Snapshot {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var head struct {
		AppData struct {
			SDK string `json:"sdk"`
		} `json:"appData"`
	}
	_ = json.Unmarshal(raw, &head)
	return &Snapshot{
		ThreadID: t.run.ThreadID, OrgID: t.run.OrgID, ProjectID: t.run.ProjectID, AgentVersionID: t.run.AgentVersionID,
		SDKVersion: head.AppData.SDK, Blob: raw,
	}
}

func transientStatus(err error) bool {
	se, ok := errors.AsType[*runtime.StatusError](err)
	return ok && (se.Status == 502 || se.Status == 504)
}
