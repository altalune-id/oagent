package channel_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	channelv1 "altalune.id/oagent/gen/go/channel/v1"
	"altalune.id/oagent/internal/channel"
)

func TestDefaults_Published(t *testing.T) {
	assert.Equal(t, 3*time.Second, channel.DefaultQuietWindow)
	assert.Equal(t, 10*time.Second, channel.DefaultQuietMax)
	assert.Equal(t, 10*time.Second, channel.MaxQuiet)
	assert.LessOrEqual(t, channel.DefaultQuietWindow, channel.DefaultQuietMax)
	assert.LessOrEqual(t, channel.DefaultQuietMax, channel.MaxQuiet)
	assert.Equal(t, "Sorry, I couldn't understand that voice message. Please type your message instead.", channel.DefaultReplyVoice)
}

// SECURITY: the proto rules repeat the quiet cap; they must equal channel.MaxQuiet on create and on update.
func TestDefaults_QuietProtoRulesMatchMaxQuiet(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	maxMS := int32(channel.MaxQuiet.Milliseconds())
	names := func(err error) string {
		var verr *protovalidate.ValidationError
		if !errors.As(err, &verr) {
			return ""
		}
		var b strings.Builder
		for _, viol := range verr.Violations {
			b.WriteString(protovalidate.FieldPathString(viol.Proto.GetField()) + " ")
		}
		return b.String()
	}
	for _, ms := range []int32{0, maxMS} {
		assert.NotContains(t, names(v.Validate(&channelv1.CreateConnectionRequest{QuietWindowMs: &ms, QuietMaxMs: &ms})), "quiet", "%d", ms)
		assert.NotContains(t, names(v.Validate(&channelv1.UpdateConnectionRequest{QuietWindowMs: &ms, QuietMaxMs: &ms})), "quiet", "%d", ms)
	}
	for _, ms := range []int32{-1, maxMS + 1} {
		assert.Contains(t, names(v.Validate(&channelv1.CreateConnectionRequest{QuietWindowMs: &ms, QuietMaxMs: &ms})), "quiet_window_ms", "%d", ms)
		assert.Contains(t, names(v.Validate(&channelv1.CreateConnectionRequest{QuietMaxMs: &ms})), "quiet_max_ms", "%d", ms)
		assert.Contains(t, names(v.Validate(&channelv1.UpdateConnectionRequest{QuietWindowMs: &ms, QuietMaxMs: &ms})), "quiet_window_ms", "%d", ms)
		assert.Contains(t, names(v.Validate(&channelv1.UpdateConnectionRequest{QuietMaxMs: &ms})), "quiet_max_ms", "%d", ms)
	}
}
