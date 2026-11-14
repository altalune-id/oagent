package run

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ContentPart is one AG-UI content part of a stored user message; ID is the channel message id and never reaches the runtime.
type ContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	Source   *MediaSource  `json:"source,omitempty"`
	ID       string        `json:"id,omitempty"`
	Metadata *partMetadata `json:"metadata,omitempty"`
}

// MediaSource is an AG-UI data source; a stored part leaves Value empty and the bytes stay with the channel.
type MediaSource struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	MimeType string `json:"mimeType"`
}

type partMetadata struct {
	Transcript *transcript `json:"transcript,omitempty"`
}

type transcript struct {
	Status  string `json:"status"`
	Text    string `json:"text,omitempty"`
	Seconds int64  `json:"seconds,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

const (
	transcriptDone    = "done"
	transcriptFailed  = "failed"
	transcriptSkipped = "skipped"
)

// ImagePart is the stored part of an inbound photo, fetched from the channel by id when a run reads it.
func ImagePart(id, mime string) ContentPart {
	return ContentPart{Type: "image", Source: &MediaSource{Type: "data", MimeType: mime}, ID: id}
}

// VoiceNotePart is the stored part of an inbound voice note, fetched from the channel by id when a run transcribes it.
func VoiceNotePart(id, mime string) ContentPart {
	return ContentPart{Type: "audio", Source: &MediaSource{Type: "data", MimeType: mime}, ID: id}
}

func (p ContentPart) transcriptOf() *transcript {
	if p.Metadata == nil {
		return nil
	}
	return p.Metadata.Transcript
}

func (p ContentPart) heard() string {
	if tr := p.transcriptOf(); tr != nil && tr.Status == transcriptDone {
		return tr.Text
	}
	return ""
}

func partText(p ContentPart) string {
	switch p.Type {
	case "text":
		return p.Text
	case "audio":
		if text := p.heard(); text != "" {
			return noteVoice + " " + text
		}
		return noteVoice
	case "image":
		return notePhoto
	}
	return ""
}

func withTranscript(body json.RawMessage, partIndex int, tr transcript) (json.RawMessage, error) {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("run: message body: %w", err)
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(msg["content"], &parts); err != nil {
		return nil, fmt.Errorf("run: message content: %w", err)
	}
	if partIndex < 0 || partIndex >= len(parts) {
		return nil, fmt.Errorf("run: no content part %d", partIndex)
	}
	part := parts[partIndex]
	var kind string
	if json.Unmarshal(part["type"], &kind) != nil || kind != "audio" {
		return nil, errors.New("run: content part is not audio")
	}
	meta := map[string]json.RawMessage{}
	if raw, ok := part["metadata"]; ok {
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, fmt.Errorf("run: part metadata: %w", err)
		}
	}
	if meta == nil {
		meta = map[string]json.RawMessage{}
	}
	var err error
	if meta["transcript"], err = json.Marshal(tr); err != nil {
		return nil, err
	}
	if part["metadata"], err = json.Marshal(meta); err != nil {
		return nil, err
	}
	if msg["content"], err = json.Marshal(parts); err != nil {
		return nil, err
	}
	return json.Marshal(msg)
}
