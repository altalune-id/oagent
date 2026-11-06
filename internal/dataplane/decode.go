package dataplane

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"altalune.id/oagent/apperror"
)

const (
	maxBody    = 64 << 10
	maxMessage = 32 << 10
)

//nolint:gochecknoglobals // the fixed id charset of threadId, Oagent-User and Idempotency-Key.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,128}$`)

type runAgentInput struct {
	ThreadID        string            `json:"threadId"`
	RunID           json.RawMessage   `json:"runId"`
	ParentRunID     json.RawMessage   `json:"parentRunId"`
	State           json.RawMessage   `json:"state"`
	Messages        []json.RawMessage `json:"messages"`
	Tools           json.RawMessage   `json:"tools"`
	Context         json.RawMessage   `json:"context"`
	ForwardedProps  json.RawMessage   `json:"forwardedProps"`
	ProtocolVersion json.RawMessage   `json:"protocolVersion"`
	Resume          json.RawMessage   `json:"resume"`
}

type lastMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type turnInput struct {
	ThreadID  string
	MessageID string
	Text      string
}

func invalid(field string, reason apperror.FieldReason) error {
	return apperror.Validation([]apperror.Violation{{Field: field, Reason: reason}})
}

func decodeInput(r *http.Request) (turnInput, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return turnInput{}, withStatus(http.StatusRequestEntityTooLarge, errBodyTooLarge)
		}
		return turnInput{}, invalid("body", apperror.FieldInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var in runAgentInput
	if err := dec.Decode(&in); err != nil {
		field := "body"
		if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			field = strings.Trim(name, `"`)
			return turnInput{}, invalid(field, apperror.FieldNotAllowed)
		}
		return turnInput{}, invalid(field, apperror.FieldInvalid)
	}
	if !idPattern.MatchString(in.ThreadID) {
		return turnInput{}, invalid("threadId", apperror.FieldInvalidFormat)
	}
	if len(in.Messages) == 0 {
		return turnInput{}, invalid("messages", apperror.FieldRequired)
	}
	var last lastMessage
	if err := json.Unmarshal(in.Messages[len(in.Messages)-1], &last); err != nil || last.Role != "user" {
		return turnInput{}, invalid("messages", apperror.FieldInvalid)
	}
	text, ok := textOf(last.Content)
	if !ok || strings.TrimSpace(text) == "" {
		return turnInput{}, invalid("messages.content", apperror.FieldRequired)
	}
	if len(text) > maxMessage || !utf8.ValidString(text) {
		return turnInput{}, invalid("messages.content", apperror.FieldTooLong)
	}
	return turnInput{ThreadID: in.ThreadID, MessageID: last.ID, Text: text}, nil
}

func textOf(content json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text, true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type != "text" {
			return "", false
		}
		b.WriteString(p.Text)
	}
	return b.String(), true
}

func requestHash(in turnInput) []byte {
	canonical, _ := json.Marshal(struct {
		ThreadID string `json:"threadId"`
		Content  string `json:"content"`
	}{in.ThreadID, in.Text})
	sum := sha256.Sum256(canonical)
	return sum[:]
}
