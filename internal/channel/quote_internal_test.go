package channel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
)

func TestInboxMessage_KeepsTheQuotedID(t *testing.T) {
	in := Inbound{ExternalID: "wamid-2", Type: "text", Text: "ya", QuotedID: "wamid-prompt", Time: time.Now()}
	m, err := inboxMessage(in, in.Text, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "wamid-prompt", m.QuotedID(), "the group decision rule reads the quoted WhatsApp id")

	in.QuotedID = ""
	m, err = inboxMessage(in, in.Text, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, m.QuotedID())
	assert.NotContains(t, string(m.Body), "quoted")
}

func TestInboxMessage_VoiceNoteIsAnAudioPart(t *testing.T) {
	voice := Inbound{ExternalID: "wamid-3", Type: "audio", Time: time.Now(), Media: &MediaRef{MessageID: "msg_1", Mime: "audio/ogg", Voice: true}}
	m, err := inboxMessage(voice, "", uuid.New())
	require.NoError(t, err)
	assert.JSONEq(t, `[{"type":"audio","source":{"type":"data","value":"","mimeType":"audio/ogg"},"id":"wamid-3"}]`, content(t, m))
	assert.Contains(t, string(m.Body), `"voice":true`, "the channel note still marks the voice note")

	m, err = inboxMessage(voice, "dengar", uuid.New())
	require.NoError(t, err)
	assert.JSONEq(t, `[{"type":"audio","source":{"type":"data","value":"","mimeType":"audio/ogg"},"id":"wamid-3"},{"type":"text","text":"dengar"}]`,
		content(t, m), "the caption is kept after the audio")

	music := voice
	music.Media = &MediaRef{MessageID: "msg_2", Mime: "audio/mpeg"}
	m, err = inboxMessage(music, "", uuid.New())
	require.NoError(t, err)
	assert.JSONEq(t, `"[audio]"`, content(t, m), "audio that is not a voice note reads as today")
}

func content(t *testing.T, m run.Message) string {
	t.Helper()
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(m.Body, &body))
	return string(body.Content)
}
