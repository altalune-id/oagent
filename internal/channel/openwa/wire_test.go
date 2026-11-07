package openwa

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strictDecode(t *testing.T, raw []byte, into any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(into))
}

func TestFixtures_MatchOpenWAShapes(t *testing.T) {
	events, err := filepath.Glob(filepath.Join("testdata", "message_matched_*.json"))
	require.NoError(t, err)
	require.Len(t, events, 11, "text, image, voice, video, document, location, contact, group mention, group members, hidden phone and reaction")
	for _, path := range events {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			var ev messageEvent
			strictDecode(t, raw, &ev)
			again, err := json.Marshal(ev)
			require.NoError(t, err)
			assert.JSONEq(t, string(raw), string(again), "every field of the payload has a home in the struct, and nothing more")
			assert.NotNil(t, ev.Matched, "a matched event names its reasons")
			assert.NotNil(t, ev.Message.Mentions, "mentions is never null")
			assert.NotEmpty(t, ev.Sender.JID)
			assert.False(t, ev.Message.Timestamp.IsZero(), "the WhatsApp time parses")
		})
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "envelope_message_received.json"))
	require.NoError(t, err)
	var env envelope
	strictDecode(t, raw, &env)
	assert.Equal(t, "message.received", env.Type)
	assert.Equal(t, "v1", env.APIVersion)
	var data messageEvent
	strictDecode(t, env.Data, &data)
	assert.Nil(t, data.Matched, "a received event has no reasons")
	dataAgain, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, string(env.Data), string(dataAgain), "a received event carries no matched key at all")
	again, err := json.Marshal(env)
	require.NoError(t, err)
	assert.JSONEq(t, string(raw), string(again))
}

func TestFixtures_CoverTheShapesTheProviderHandles(t *testing.T) {
	load := func(name string) messageEvent {
		raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
		require.NoError(t, err)
		var ev messageEvent
		strictDecode(t, raw, &ev)
		return ev
	}
	assert.Equal(t, "image", load("message_matched_image").Message.Type)
	assert.NotNil(t, load("message_matched_image").Message.Media)
	voice := load("message_matched_voice")
	require.NotNil(t, voice.Message.Media)
	assert.True(t, voice.Message.Media.Voice)
	group := load("message_matched_group_mention")
	assert.Equal(t, "group", group.Chat.Kind)
	assert.Equal(t, []string{"group_mention"}, group.Matched.Reasons)
	hidden := load("message_matched_hidden_phone")
	assert.Empty(t, hidden.Sender.Phone, "WhatsApp hid the number")
	assert.NotEmpty(t, hidden.Sender.LID)
	reaction := load("message_matched_reaction")
	assert.Equal(t, "reaction", reaction.Message.Type)
	require.NotNil(t, reaction.Message.Quoted)
}
