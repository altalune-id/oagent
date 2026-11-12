package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	approvalInterrupt   = "oagent:approval"
	toolNameSeparator   = "__"
	approvalExpiredText = "This approval expired"
	approvalPending     = "pending"
	callNotApprovedText = "This call was not approved"
	callUnknownText     = "the outcome of this call is unknown after a restart; check before trying again"
)

// SECURITY: the v3 reason names the call (toolUseId, server, tool); the id is opaque, echoed on resume; anything odd fails closed.
func approvalCalls(interrupts []runtime.Interrupt) ([]ApprovalCall, error) {
	if len(interrupts) == 0 {
		return nil, fmt.Errorf("run: awaiting approval with no interrupt")
	}
	out := make([]ApprovalCall, 0, len(interrupts))
	seenIDs, seenCalls := map[string]bool{}, map[string]bool{}
	for _, in := range interrupts {
		if in.Name != approvalInterrupt || in.ID == "" {
			return nil, fmt.Errorf("run: unknown interrupt %q named %q", in.ID, in.Name)
		}
		var reason struct {
			ToolUseID string          `json:"toolUseId"`
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Input     json.RawMessage `json:"input"`
			ArgsHash  string          `json:"argsHash"`
			Keys      []string        `json:"keys"`
		}
		if err := json.Unmarshal(in.Reason, &reason); err != nil {
			return nil, fmt.Errorf("run: interrupt %q reason: %w", in.ID, err)
		}
		if reason.ToolUseID == "" || reason.Server == "" || reason.Tool == "" || reason.ArgsHash == "" {
			return nil, fmt.Errorf("run: interrupt %q has no tool call id, server, tool or hash", in.ID)
		}
		if seenIDs[in.ID] || seenCalls[reason.ToolUseID] {
			return nil, fmt.Errorf("run: interrupt %q raised twice", in.ID)
		}
		seenIDs[in.ID], seenCalls[reason.ToolUseID] = true, true
		out = append(out, ApprovalCall{
			ToolCallID: reason.ToolUseID, InterruptID: in.ID, Server: reason.Server, Tool: reason.Tool,
			Args: reason.Input, ArgsHash: reason.ArgsHash, Keys: reason.Keys,
		})
	}
	return out, nil
}

// ResumeResponses answers every call of a decided approval, in call order; the snapshot hash check binds them to the open interrupts.
func ResumeResponses(a *RunApproval) ([]runtime.InterruptResponse, error) {
	approved, text := false, a.DecisionText
	switch a.Status {
	case "approved":
		approved, text = true, ""
	case "expired":
		text = approvalExpiredText
	case "rejected":
	default:
		return nil, fmt.Errorf("run: approval is %s, not decided", a.Status)
	}
	out := make([]runtime.InterruptResponse, 0, len(a.Calls))
	for _, c := range a.Calls {
		ok, why := approved, text
		switch {
		case approved && c.State == "unknown":
			// NOTE: the runtime denies with this text before any replay, so it is what the model reads for a call that may have run.
			ok, why = false, callUnknownText
		case approved && !runnable(c.State):
			// SECURITY: a resume refuses a call that is not approved or executed.
			ok, why = false, callNotApprovedText
		}
		body, err := json.Marshal(map[string]any{"approved": ok, "text": why, "argsHash": c.ArgsHash})
		if err != nil {
			return nil, err
		}
		out = append(out, runtime.InterruptResponse{InterruptID: c.InterruptID, Response: body})
	}
	return out, nil
}

// SECURITY: answers go only with the snapshot the interrupt was raised in; a read error is returned, never taken as missing.
func (s *Service) raisedSnapshot(ctx context.Context, t *turn) (*Snapshot, bool, error) {
	if s.snaps == nil {
		return nil, false, nil
	}
	snap, err := s.snaps.Load(ctx, t.run.ThreadID)
	if err != nil || snap == nil {
		return nil, false, err
	}
	return snap, bytes.Equal(SnapshotHash(snap), t.approval.RaisedSnapshotHash), nil
}

// NOTE: pendingDecision expires a pending approval past its expiry on the database clock, else tries the chat; nil means it still waits.
func (s *Service) pendingDecision(ctx context.Context, t *turn, a *RunApproval) (*RunApproval, error) {
	expired, err := s.approvals.Expire(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if !expired {
		// NOTE: re-read whatever the chat did: a console decision that won the race resumes now, not at the next reconcile.
		if err := s.decideFromChat(ctx, t, a); err != nil {
			return nil, err
		}
	}
	latest, err := s.approvals.Latest(ctx, t.run.ID)
	if err != nil || latest == nil || latest.Status == approvalPending {
		return nil, err
	}
	return latest, nil
}

func runnable(state string) bool {
	return state == "approved" || state == "executing" || state == "executed"
}

// SECURITY: mayHaveRun reports a call that is executing, executed or unknown; such an approval was used and never expires.
func mayHaveRun(a *RunApproval) bool {
	return len(callIDs(a.Calls, "executing"))+len(callIDs(a.Calls, "executed"))+len(callIDs(a.Calls, "unknown")) > 0
}

// NOTE: voidable is a decided approval none of whose calls may have run, so its run's end expires it, keeping the decider.
func voidable(a *RunApproval) bool {
	return a != nil && a.ID != uuid.Nil && !mayHaveRun(a)
}

func callIDs(calls []ApprovalCall, state string) []string {
	var out []string
	for _, c := range calls {
		if c.State == state {
			out = append(out, c.ToolCallID)
		}
	}
	return out
}

func (a *RunApproval) setState(toolCallID, state string) {
	for i := range a.Calls {
		if a.Calls[i].ToolCallID == toolCallID {
			a.Calls[i].State = state
		}
	}
}

func (a *RunApproval) state(toolCallID string) string {
	for _, c := range a.Calls {
		if c.ToolCallID == toolCallID {
			return c.State
		}
	}
	return ""
}

// SECURITY: approved calls become executing in a committed unit of work that re-checks the lease, right before the request leaves Go.
func (s *Service) markExecuting(ctx context.Context, t *turn) error {
	a := t.approval
	if a == nil || a.Status != "approved" {
		return nil
	}
	ids := callIDs(a.Calls, "approved")
	if len(ids) == 0 {
		return nil
	}
	if err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.HoldLease(ctx, t.token, t.run.ThreadID); err != nil {
			return err
		}
		return s.approvals.MarkExecuting(ctx, a.ID, ids)
	}); err != nil {
		return err
	}
	for _, id := range ids {
		a.setState(id, "executing")
	}
	t.marked = ids
	return nil
}

// NOTE: undelivered puts this request's executing calls back to approved when the request provably never reached the runtime.
func (s *Service) undelivered(ctx context.Context, t *turn, cause error) error {
	if len(t.marked) == 0 || !neverDelivered(cause) {
		return nil
	}
	if err := s.approvals.MarkUndelivered(ctx, t.approval.ID, t.marked); err != nil {
		return err
	}
	for _, id := range t.marked {
		t.approval.setState(id, "approved")
	}
	t.marked = nil
	return nil
}

// SECURITY: an allow-list: only a dial or DNS failure, or a runtime pre-turn status, proves the request never ran.
func neverDelivered(err error) bool {
	if op, ok := errors.AsType[*net.OpError](err); ok && op.Op == "dial" {
		return true
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	if se, ok := errors.AsType[*runtime.StatusError](err); ok {
		return slices.Contains(preTurnStatuses(), se.Status)
	}
	if ce, ok := errors.AsType[*runtime.ContractError](err); ok {
		return slices.Contains(preTurnStatuses(), ce.Status)
	}
	return runtime.IsBusyError(err) || runtime.IsSnapshotUnloadableError(err)
}

// NOTE: preTurnStatuses are the statuses the runtime answers from its own checks, before it starts a turn.
func preTurnStatuses() []int { return []int{400, 401, 404, 405, 408, 413, 422, 503} }

// SECURITY: after a crash an executing call with a stored result is replayed (executed); one without becomes unknown and never runs again.
func (s *Service) settleExecuting(ctx context.Context, t *turn) error {
	a := t.approval
	if a == nil {
		return nil
	}
	executing := callIDs(a.Calls, "executing")
	if len(executing) == 0 {
		return nil
	}
	stored, err := s.store.StoredResults(ctx, t.run.ID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, sr := range stored {
		have[sr.ToolUseID] = true
	}
	for _, id := range executing {
		mark, state := s.approvals.MarkUnknown, "unknown"
		if have[id] {
			mark, state = s.approvals.MarkExecuted, "executed"
		}
		if err := mark(ctx, a.ID, id); err != nil {
			return err
		}
		a.setState(id, state)
	}
	return nil
}

// NOTE: checkpoint stores a tool result and, for a call this turn's approval made executing, marks it executed in the same unit of work.
func (s *Service) checkpoint(ctx context.Context, t *turn, toolUseID string, msgs []Message) error {
	a := t.approval
	if a == nil || a.state(toolUseID) != "executing" {
		return s.store.PutCheckpoint(ctx, t.token, t.run.ID, toolUseID, msgs)
	}
	if err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.PutCheckpoint(ctx, t.token, t.run.ID, toolUseID, msgs); err != nil {
			return err
		}
		return s.approvals.MarkExecuted(ctx, a.ID, toolUseID)
	}); err != nil {
		return err
	}
	a.setState(toolUseID, "executed")
	return nil
}
