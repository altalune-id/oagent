package run

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"
)

const imageUnsupported = "image_unsupported"

func (s *Service) reply(t *turn, final []Message) []Message {
	r := t.run
	if t.thread.ConnectionID == uuid.Nil {
		return final
	}
	group := t.thread.Kind == threadKindGroup
	sr := setReplies(t.taken, t.thread.Kind, t.model.Vision)
	switch r.Status {
	case StatusCompleted, StatusLimitReached:
		if sr.voice {
			return withSetReply(r, final, t.texts.Voice)
		}
		if i := lastText(final); i >= 0 {
			if sr.photoUnreadable {
				final[i] = appendText(final[i], t.texts.PhotoUnreadable)
			}
			r.ReplyState = ReplyDue
			return final
		}
		if sr.photoUnreadable && !group {
			return withSetReply(r, final, t.texts.PhotoUnreadable)
		}
		r.ReplyState = ReplyNone
	case StatusFailed:
		switch {
		case group:
			r.ReplyState = ReplyNone
		case t.imageRefused:
			return withSetReply(r, final, t.texts.PhotoUnreadable)
		default:
			return withSetReply(r, final, t.texts.Failure)
		}
	case StatusCancelled, StatusAwaitingApproval:
		r.ReplyState = ReplyNone
	}
	return final
}

func withSetReply(r *Run, final []Message, text string) []Message {
	if text == "" {
		r.ReplyState = ReplyNone
		return final
	}
	r.ReplyState = ReplyDue
	body, _ := json.Marshal(map[string]string{"id": uuid.NewString(), "role": string(RoleAssistant), "content": text})
	return append(final, Message{Role: RoleAssistant, Body: body})
}

// LastText is the text of the last assistant message in msgs with string content; ok is false when there is none.
func LastText(msgs []Message) (text string, ok bool) {
	i := lastText(msgs)
	if i < 0 {
		return "", false
	}
	return textOf(msgs[i]), true
}

func lastText(msgs []Message) int {
	for i, m := range slices.Backward(msgs) {
		if m.Role == RoleAssistant && textOf(m) != "" {
			return i
		}
	}
	return -1
}

func textOf(m Message) string {
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	var text string
	if json.Unmarshal(m.Body, &body) != nil || json.Unmarshal(body.Content, &text) != nil {
		return ""
	}
	return text
}

func appendText(m Message, note string) Message {
	var body map[string]json.RawMessage
	if note == "" || json.Unmarshal(m.Body, &body) != nil {
		return m
	}
	body["content"] = quote(textOf(m) + "\n\n" + note)
	m.Body, _ = json.Marshal(body)
	return m
}
