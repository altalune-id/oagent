package run_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

var setTexts = run.ReplyTexts{Failure: "Maaf, ada gangguan.", Voice: "Pesan suara belum didukung.", PhotoUnreadable: "Foto belum bisa dibaca."}

func (f *svcFixture) withTexts(conn uuid.UUID) {
	f.store.Texts = map[uuid.UUID]run.ReplyTexts{conn: setTexts}
}

func (f *svcFixture) assistantTexts(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, m := range f.store.Messages {
		if m.ThreadID != f.thread || m.Role != run.RoleAssistant {
			continue
		}
		var body struct {
			Content json.RawMessage `json:"content"`
		}
		require.NoError(t, json.Unmarshal(m.Body, &body))
		var s string
		if json.Unmarshal(body.Content, &s) == nil && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (f *svcFixture) last(t *testing.T) string {
	t.Helper()
	all := f.assistantTexts(t)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

func (f *svcFixture) stored(r *run.Run) *run.Run { return f.store.Runs[r.ID] }

func TestReply_CompletedDMIsDue(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.say(t, "halo")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState)
	assert.NotEmpty(t, f.last(t))
}

func TestReply_APIThreadHasNoReplyState(t *testing.T) {
	f := newSvc(t)
	f.say(t, "halo")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Empty(t, f.stored(r).ReplyState)
}

func TestReply_FailureTextDMSilentGroup(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.say(t, "halo")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "model_error")})
	require.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState)
	assert.Equal(t, setTexts.Failure, f.last(t), "the failure text is stored as the run's reply")

	g := newSvc(t)
	_, conn = g.channel(t, "group", true)
	g.withTexts(conn)
	g.say(t, "halo")
	r = g.execute(t, fakes.RuntimeReply{Stream: stream(t, "model_error")})
	assert.Equal(t, run.ReplyNone, g.stored(r).ReplyState)
	assert.NotContains(t, g.assistantTexts(t), setTexts.Failure, "a group stays silent")
}

func TestReply_ImageUnsupportedUsesPhotoText(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.post(t, photoMsg("struk", "wamid-1", nil))
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "image_refused")})
	require.Equal(t, run.StatusFailed, r.Status)
	assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState)
	assert.Equal(t, setTexts.PhotoUnreadable, f.last(t))
}

func TestReply_CancelSendsNothing(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.say(t, "halo")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: cut(stream(t, "plain_reply")), Block: true})
	token := f.claim(t)
	ctx, cancel := context.WithCancelCause(f.ctx)
	cancel(run.ErrCancelRequested)
	r, err := f.svc.Execute(ctx, f.thread, token)
	require.NoError(t, err)
	require.Equal(t, run.StatusCancelled, r.Status)
	assert.Equal(t, run.ReplyNone, f.stored(r).ReplyState)
}

func TestReply_InterruptedWaitsForResume(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.say(t, "halo")
	r := f.execute(t)
	require.Equal(t, run.StatusInterrupted, r.Status)
	assert.Empty(t, f.stored(r).ReplyState, "an interrupted run has no reply yet")
	assert.Empty(t, f.assistantTexts(t))
}

func TestReply_VoiceSetReplyStoredInDMOnly(t *testing.T) {
	f := newSvc(t)
	_, conn := f.channel(t, "dm", true)
	f.withTexts(conn)
	f.post(t, voiceMsg("wamid-1"))
	r := f.execute(t)
	assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState)
	assert.Equal(t, []string{setTexts.Voice}, f.assistantTexts(t), "the thread shows what the person received")

	g := newSvc(t)
	_, conn = g.channel(t, "group", true)
	g.withTexts(conn)
	g.post(t, voiceMsg("wamid-1"))
	r = g.execute(t)
	assert.Equal(t, run.ReplyNone, g.stored(r).ReplyState)
	assert.Empty(t, g.assistantTexts(t))
}

func TestReply_PhotoNoteAppendedToTheSentReply(t *testing.T) {
	for _, kind := range []string{"dm", "group"} {
		f := newSvc(t)
		_, conn := f.channel(t, kind, false)
		f.withTexts(conn)
		f.post(t, photoMsg("kopi", "wamid-1", nil))
		r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
		assert.Equal(t, run.ReplyDue, f.stored(r).ReplyState, kind)
		all := f.assistantTexts(t)
		require.Len(t, all, 1, "%s: one stored reply, not a second message", kind)
		assert.Regexp(t, `.+\n\n`+setTexts.PhotoUnreadable+`$`, all[0], kind)
	}
}

func TestReply_ChannelWithoutTextsLoadedInterrupts(t *testing.T) {
	f := newSvc(t)
	f.channel(t, "dm", true)
	f.store.TextsErr = assert.AnError
	f.say(t, "halo")
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusInterrupted, r.Status, "a turn never runs without its set replies")
	assert.Empty(t, f.rt.Requests)
}
