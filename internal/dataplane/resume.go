package dataplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
)

const maxResumeText = 4000

// NOTE: resumeDecision is what a resume[] answers for the thread's pending approval: one decision for every call.
type resumeDecision struct {
	Approved bool
	Text     string
}

type resumeEntry struct {
	InterruptID string          `json:"interruptId"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	Metadata    json.RawMessage `json:"metadata"`
}

type resumePayload struct {
	Approved *bool           `json:"approved"`
	Text     json.RawMessage `json:"text"`
}

// SECURITY: each open id answered exactly once; a payload key but approved and text is APR005; approves only if every entry does.
func resumeAnswer(raw json.RawMessage, open []string) (resumeDecision, error) {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return resumeDecision{}, invalid("resume", apperror.FieldInvalid)
	}
	out := resumeDecision{Approved: true}
	seen := map[string]bool{}
	for _, rawEntry := range entries {
		e, err := decodeEntry(rawEntry)
		if err != nil {
			return resumeDecision{}, err
		}
		if seen[e.InterruptID] || !slices.Contains(open, e.InterruptID) {
			return resumeDecision{}, invalid("resume.interruptId", apperror.FieldInvalid)
		}
		seen[e.InterruptID] = true
		approved, text, err := decodeAnswer(e)
		if err != nil {
			return resumeDecision{}, err
		}
		if !approved && out.Approved {
			out.Approved, out.Text = false, text
		}
	}
	var missing []string
	for _, id := range open {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return resumeDecision{}, &approval.ResumeIncompleteError{Missing: missing}
	}
	if out.Approved {
		out.Text = ""
	}
	return out, nil
}

// SECURITY: entries and payloads are read by exact key; encoding/json alone would match a key by case and keep the last duplicate.
func exactObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errNotObject
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		if _, dup := out[key]; dup {
			return nil, errDuplicateKey
		}
		out[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

var (
	errNotObject    = errors.New("dataplane: not a JSON object")
	errDuplicateKey = errors.New("dataplane: duplicate key")
)

func decodeEntry(raw json.RawMessage) (resumeEntry, error) {
	var e resumeEntry
	fields, err := exactObject(raw)
	if err != nil {
		return e, invalid("resume", apperror.FieldInvalid)
	}
	// NOTE: keys are checked in sorted order, so a body with two faults always gets the same answer.
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		switch key {
		case "interruptId":
			err = json.Unmarshal(value, &e.InterruptID)
		case "status":
			err = json.Unmarshal(value, &e.Status)
		case "payload":
			e.Payload = value
		case "metadata":
			e.Metadata = value
		default:
			return e, invalid("resume."+key, apperror.FieldNotAllowed)
		}
		if err != nil {
			return e, invalid("resume."+key, apperror.FieldInvalid)
		}
	}
	if e.InterruptID == "" {
		return e, invalid("resume.interruptId", apperror.FieldRequired)
	}
	if e.Status != "resolved" && e.Status != "cancelled" {
		return e, invalid("resume.status", apperror.FieldInvalid)
	}
	return e, nil
}

func decodeAnswer(e resumeEntry) (approved bool, text string, err error) {
	if e.Status == "cancelled" {
		return false, "", nil
	}
	fields, err := exactObject(e.Payload)
	if err != nil {
		return false, "", invalid("resume.payload", apperror.FieldInvalid)
	}
	var p resumePayload
	// NOTE: keys are checked in sorted order, so a body with two faults always gets the same answer.
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		switch key {
		case "approved":
			var b bool
			// NOTE: null decodes as false; it must stay a 400, never become a reject.
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &b) != nil {
				return false, "", invalid("resume.payload.approved", apperror.FieldInvalid)
			}
			p.Approved = &b
		case "text":
			p.Text = value
		default:
			return false, "", &approval.EditedArgsRefusedError{}
		}
	}
	if p.Approved == nil {
		return false, "", invalid("resume.payload.approved", apperror.FieldRequired)
	}
	if len(p.Text) > 0 && json.Unmarshal(p.Text, &text) != nil {
		return false, "", invalid("resume.payload.text", apperror.FieldInvalid)
	}
	if len([]rune(text)) > maxResumeText {
		return false, "", invalid("resume.payload.text", apperror.FieldTooLong)
	}
	return *p.Approved, text, nil
}
