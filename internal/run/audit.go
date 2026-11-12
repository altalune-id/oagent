package run

import (
	"context"
	"encoding/json"
)

// SECURITY: audit runs per checkpoint (so cut legs count): a tool outside the grants, or a gated call with no consumed approval, alerts.
func (s *Service) audit(ctx context.Context, t *turn, toolCallID, name string, input json.RawMessage) {
	grant, granted := t.grants[name]
	if granted && (!grant.RequireApproval || !s.needsApproval(grant, input) || consumed(t.approval, toolCallID)) {
		return
	}
	s.log.ErrorContext(ctx, "run: approval audit mismatch: a tool ran outside its grant or with no consumed approval",
		"run", t.run.PublicID, "tool", name, "call", toolCallID)
	if s.mismatches != nil {
		s.mismatches.Add(ctx, 1)
	}
}

func (s *Service) needsApproval(grant ToolGrant, args json.RawMessage) bool {
	if s.approvals == nil {
		return true
	}
	return s.approvals.NeedsApproval(grant, args)
}

func consumed(a *RunApproval, toolCallID string) bool {
	if a == nil || a.Status != "approved" {
		return false
	}
	state := a.state(toolCallID)
	return state == "executing" || state == "executed"
}
