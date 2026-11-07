package run

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func channelMsg(content any, voice bool) Message {
	body, _ := json.Marshal(map[string]any{"id": "m", "role": "user", "content": content, "channel": map[string]any{"type": "audio", "voice": voice}})
	return Message{Role: RoleUser, Body: body}
}

var photo = []map[string]any{{"type": "text", "text": "struk"}, {"type": "image", "source": map[string]string{"type": "data", "value": "", "mimeType": "image/jpeg"}}}

func TestTurn_VoiceSetReplyDMSilentGroup(t *testing.T) {
	voice := []Message{channelMsg("[voice note]", true), channelMsg("[voice note]", true)}
	assert.True(t, setReplies(voice, "dm", true).voice, "a voice-only DM turn gets the set reply")
	assert.False(t, setReplies(voice, threadKindGroup, true).voice, "a group stays silent")
	mixed := append(voice, channelMsg("halo", false))
	assert.False(t, setReplies(mixed, "dm", true).voice, "with text the model answers")
	assert.True(t, voiceOnly(voice))
	assert.False(t, voiceOnly(mixed))
	assert.False(t, voiceOnly([]Message{{Role: RoleAssistant}}), "no user message is not a voice turn")
}

func TestTurn_NonVisionPhotoGetsTheUnreadableReply(t *testing.T) {
	withPhoto := []Message{channelMsg(photo, false)}
	assert.True(t, setReplies(withPhoto, "dm", false).photoUnreadable)
	assert.True(t, setReplies(withPhoto, threadKindGroup, false).photoUnreadable, "a group appends it to a sent reply only")
	assert.False(t, setReplies(withPhoto, "dm", true).photoUnreadable, "a vision model reads it")
	assert.False(t, setReplies([]Message{channelMsg("halo", false)}, "dm", false).photoUnreadable, "no photo, no note")
}
