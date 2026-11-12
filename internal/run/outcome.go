package run

import (
	"context"
	"encoding/json"
	"time"
)

// NOTE: approvalResponseSchema is the AG-UI responseSchema of an approval interrupt: what a resume[] payload may hold.
const approvalResponseSchema = `{"type":"object","properties":{"approved":{"type":"boolean"},"text":{"type":"string"}},` +
	`"required":["approved"]}`

type interrupt struct {
	ID             string            `json:"id"`
	Reason         string            `json:"reason"`
	ToolCallID     string            `json:"toolCallId"`
	Message        string            `json:"message"`
	ExpiresAt      string            `json:"expiresAt"`
	ResponseSchema json.RawMessage   `json:"responseSchema"`
	Metadata       map[string]string `json:"metadata"`
}

// SECURITY: the outcome names each call by its tool call id; the runtime's Strands interrupt id never leaves Go.
func (s *Service) interruptOutcome(ctx context.Context, t *turn) json.RawMessage {
	if s.approvals == nil {
		return nil
	}
	a, err := s.approvals.Latest(ctx, t.run.ID)
	if err != nil || a == nil {
		s.log.WarnContext(ctx, "run: no approval for the interrupt outcome", "run", t.run.PublicID)
		return nil
	}
	out := struct {
		Type       string      `json:"type"`
		Interrupts []interrupt `json:"interrupts"`
	}{Type: "interrupt"}
	for _, c := range a.Calls {
		out.Interrupts = append(out.Interrupts, interrupt{
			ID: c.ToolCallID, Reason: "tool_call", ToolCallID: c.ToolCallID, Message: a.Preview,
			ExpiresAt: a.ExpiresAt.UTC().Format(time.RFC3339), ResponseSchema: json.RawMessage(approvalResponseSchema),
			Metadata: map[string]string{"approval": a.PublicID},
		})
	}
	raw, _ := json.Marshal(out)
	return raw
}
