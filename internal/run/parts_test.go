package run

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartText(t *testing.T) {
	voice := VoiceNotePart("wamid-1", "audio/ogg")
	done := voice
	done.Metadata = &partMetadata{Transcript: &transcript{Status: transcriptDone, Text: "beli kopi", Seconds: 3}}
	failed := voice
	failed.Metadata = &partMetadata{Transcript: &transcript{Status: transcriptFailed, Reason: "failed"}}
	for name, c := range map[string]struct {
		part ContentPart
		want string
	}{
		"text":           {ContentPart{Type: "text", Text: "halo"}, "halo"},
		"voice, not yet": {voice, "[voice note]"},
		"voice, done":    {done, "[voice note] beli kopi"},
		"voice, failed":  {failed, "[voice note]"},
		"image":          {ImagePart("wamid-2", "image/jpeg"), "[photo]"},
		"unknown":        {ContentPart{Type: "video"}, ""},
	} {
		assert.Equal(t, c.want, partText(c.part), name)
	}
}

func TestParts_JSONShape(t *testing.T) {
	raw, err := json.Marshal([]ContentPart{VoiceNotePart("wamid-1", "audio/ogg"), ImagePart("wamid-2", "image/jpeg")})
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"audio","source":{"type":"data","value":"","mimeType":"audio/ogg"},"id":"wamid-1"},
		{"type":"image","source":{"type":"data","value":"","mimeType":"image/jpeg"},"id":"wamid-2"}
	]`, string(raw))
}

const voiceBody = `{
	"id": "msg-1", "role": "user", "extra": {"kept": true},
	"content": [
		{"type": "audio", "source": {"type": "data", "value": "", "mimeType": "audio/ogg"}, "id": "wamid-1", "metadata": {"other": 1}, "x": 2},
		{"type": "text", "text": "dengar ini"},
		{"type": "image", "source": {"type": "data", "value": "", "mimeType": "image/jpeg"}, "id": "wamid-2"}
	],
	"channel": {"type": "audio", "voice": true, "quoted": "wamid-0"}
}`

func TestWithTranscript_PatchesOnlyTheTranscript(t *testing.T) {
	got, err := withTranscript(json.RawMessage(voiceBody), 0, transcript{Status: transcriptDone, Text: "beli kopi", Seconds: 3})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "msg-1", "role": "user", "extra": {"kept": true},
		"content": [
			{"type": "audio", "source": {"type": "data", "value": "", "mimeType": "audio/ogg"}, "id": "wamid-1",
			 "metadata": {"other": 1, "transcript": {"status": "done", "text": "beli kopi", "seconds": 3}}, "x": 2},
			{"type": "text", "text": "dengar ini"},
			{"type": "image", "source": {"type": "data", "value": "", "mimeType": "image/jpeg"}, "id": "wamid-2"}
		],
		"channel": {"type": "audio", "voice": true, "quoted": "wamid-0"}
	}`, string(got))

	got, err = withTranscript(got, 0, transcript{Status: transcriptSkipped, Reason: "too_long"})
	require.NoError(t, err)
	var back struct {
		Content []ContentPart `json:"content"`
	}
	require.NoError(t, json.Unmarshal(got, &back))
	assert.Equal(t, &transcript{Status: transcriptSkipped, Reason: "too_long"}, back.Content[0].Metadata.Transcript, "a later patch replaces the transcript")
}

func TestWithTranscript_Refuses(t *testing.T) {
	for name, c := range map[string]struct {
		body  string
		index int
	}{
		"not an audio part": {voiceBody, 1},
		"out of range":      {voiceBody, 3},
		"negative":          {voiceBody, -1},
		"string content":    {`{"id":"m","role":"user","content":"[voice note]"}`, 0},
		"not json":          {`nope`, 0},
	} {
		_, err := withTranscript(json.RawMessage(c.body), c.index, transcript{Status: transcriptDone, Text: "x"})
		assert.Error(t, err, name)
	}
}

func voicePartMsg(tr *transcript, caption string) Message {
	p := VoiceNotePart("wamid-1", "audio/ogg")
	if tr != nil {
		p.Metadata = &partMetadata{Transcript: tr}
	}
	parts := []ContentPart{p}
	if caption != "" {
		parts = append(parts, ContentPart{Type: "text", Text: caption})
	}
	return channelMsg(parts, true)
}

func TestVoiceOnly_CountsOnlyUntranscribedVoiceNotes(t *testing.T) {
	assert.True(t, voiceOnly([]Message{channelMsg("[voice note]", true)}), "an old string voice note")
	assert.True(t, voiceOnly([]Message{voicePartMsg(nil, "")}), "not transcribed yet")
	assert.True(t, voiceOnly([]Message{voicePartMsg(&transcript{Status: transcriptFailed, Reason: "failed"}, "")}), "transcription failed")
	assert.True(t, voiceOnly([]Message{voicePartMsg(&transcript{Status: transcriptSkipped, Reason: "too_long"}, "")}))
	assert.False(t, voiceOnly([]Message{voicePartMsg(&transcript{Status: transcriptDone, Text: "beli kopi"}, "")}), "a transcript is something to answer")
	assert.False(t, voiceOnly([]Message{voicePartMsg(nil, ""), voicePartMsg(&transcript{Status: transcriptDone, Text: "x"}, "")}))
	assert.False(t, voiceOnly([]Message{channelMsg("halo", false)}))
	assert.False(t, setReplies([]Message{voicePartMsg(nil, "")}, threadKindGroup, true).voice, "the set reply stays DM-only")
	assert.True(t, setReplies([]Message{voicePartMsg(nil, "")}, "dm", true).voice)
}

func TestPartNotes_VoiceNoteInHistory(t *testing.T) {
	m := voicePartMsg(&transcript{Status: transcriptDone, Text: "beli kopi"}, "")
	var u agUIMessage
	require.NoError(t, json.Unmarshal(m.Body, &u))
	text, ok := partNotes(u.Content)
	require.True(t, ok)
	assert.Equal(t, "[voice note] beli kopi", text)

	text, _ = partNotes(json.RawMessage(`[{"type":"audio","id":"w"},{"type":"text","text":"cap"},{"type":"image","id":"i"}]`))
	assert.Equal(t, "[voice note]\ncap\n[photo]", text)
}

func TestUserText_ReadsTheTranscript(t *testing.T) {
	assert.Equal(t, "[voice note] beli kopi\ndengar", userText(voicePartMsg(&transcript{Status: transcriptDone, Text: "beli kopi"}, "dengar")))
	assert.Equal(t, "struk", userText(channelMsg(photo, false)), "a photo adds nothing to the search text")
}
