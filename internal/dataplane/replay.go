package dataplane

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
)

const replayPage = maxPage

type settledReplay struct{ run *run.Run }

func (*settledReplay) Error() string { return "dataplane: the idempotent request already settled" }

type storedMessage struct {
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"toolCallId"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"toolCalls"`
}

func replayEvents(threadKey string, r *run.Run, msgs []run.Message) []runtime.Event {
	out := []runtime.Event{&runtime.RunStarted{Type: runtime.EventRunStarted, ThreadID: threadKey, RunID: r.PublicID}}
	for _, m := range msgs {
		var sm storedMessage
		if json.Unmarshal(m.Body, &sm) != nil {
			continue
		}
		switch sm.Role {
		case "assistant":
			for _, c := range sm.ToolCalls {
				out = append(out,
					&runtime.ToolCallStart{Type: runtime.EventToolCallStart, ToolCallID: c.ID, ToolCallName: c.Function.Name, ParentMessageID: sm.ID},
					&runtime.ToolCallArgs{Type: runtime.EventToolCallArgs, ToolCallID: c.ID, Delta: c.Function.Arguments},
					&runtime.ToolCallEnd{Type: runtime.EventToolCallEnd, ToolCallID: c.ID})
			}
			if text, ok := textOf(sm.Content); ok && text != "" {
				out = append(out,
					&runtime.TextMessageStart{Type: runtime.EventTextMessageStart, MessageID: sm.ID, Role: "assistant"},
					&runtime.TextMessageContent{Type: runtime.EventTextMessageContent, MessageID: sm.ID, Delta: text},
					&runtime.TextMessageEnd{Type: runtime.EventTextMessageEnd, MessageID: sm.ID})
			}
		case "tool":
			out = append(out, &runtime.ToolCallResult{
				Type: runtime.EventToolCallResult, MessageID: sm.ID, ToolCallID: sm.ToolCallID, Content: sm.Content, Role: "tool",
			})
		}
	}
	switch r.Status {
	case run.StatusCompleted, run.StatusLimitReached:
		out = append(out, &runtime.RunFinished{Type: runtime.EventRunFinished, ThreadID: threadKey, RunID: r.PublicID})
	case run.StatusCancelled:
		out = append(out, &runtime.RunError{Type: runtime.EventRunError, Message: "The run was cancelled.", Code: "cancelled"})
	default:
		out = append(out, &runtime.RunError{Type: runtime.EventRunError, Message: "The run failed.", Code: r.ErrorCode})
	}
	return out
}

func (h *Handler) replay(ctx context.Context, w http.ResponseWriter, threadKey string, threadID uuid.UUID, r *run.Run) error {
	var mine []run.Message
	after := int64(0)
	for {
		page, err := h.d.Store.ThreadMessages(ctx, threadID, after, replayPage)
		if err != nil {
			return err
		}
		for _, m := range page {
			if m.RunID != nil && *m.RunID == r.ID {
				mine = append(mine, m)
			}
			after = m.Seq
		}
		if len(page) < replayPage {
			break
		}
	}
	streamHeaders(w)
	sse := NewSSEWriter(w)
	for _, e := range replayEvents(threadKey, r, mine) {
		if err := sse.Event(e); err != nil {
			return err
		}
	}
	return nil
}

func streamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
}
