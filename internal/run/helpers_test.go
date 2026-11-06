package run_test

import (
	"encoding/json"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/run"
)

func userMsg(text, ext string) run.Message {
	body, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": "user", "content": text})
	return run.Message{Role: run.RoleUser, Body: body, ExternalMessageID: ext}
}

func toolMsg(callID string) run.Message {
	body, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": "tool", "toolCallId": callID, "content": "42"})
	return run.Message{Role: run.RoleTool, Body: body, ToolCallID: callID}
}

func assistantMsg(text string) run.Message {
	body, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": "assistant", "content": text})
	return run.Message{Role: run.RoleAssistant, Body: body}
}
