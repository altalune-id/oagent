package run_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

var jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}

type fetchCall struct {
	conn     uuid.UUID
	id, mime string
	maxBytes int64
}

type media struct {
	mu      sync.Mutex
	fetched []fetchCall
	read    []string
	data    []byte
	err     error
	readErr error
}

func (m *media) Fetch(_ context.Context, conn uuid.UUID, id, mime string, maxBytes int64) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetched = append(m.fetched, fetchCall{conn: conn, id: id, mime: mime, maxBytes: maxBytes})
	if m.err != nil {
		return nil, "", m.err
	}
	return m.data, mime, nil
}

func (m *media) MarkRead(_ context.Context, _ uuid.UUID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.read = append(m.read, id)
	return m.readErr
}

func (m *media) ids() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, c := range m.fetched {
		out = append(out, c.id)
	}
	return out
}

type part struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Source *struct {
		Type, Value, MimeType string
	} `json:"source"`
	ID *string `json:"id"`
}

func photoMsg(caption, ext string, speaker *uuid.UUID) run.Message {
	parts := []map[string]any{}
	if caption != "" {
		parts = append(parts, map[string]any{"type": "text", "text": caption})
	}
	parts = append(parts, map[string]any{"type": "image", "source": map[string]string{"type": "data", "value": "", "mimeType": "image/jpeg"}, "id": ext})
	body, _ := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": parts, "channel": map[string]any{"type": "image"}})
	return run.Message{Role: run.RoleUser, Body: body, ExternalMessageID: ext, SpeakerID: speaker}
}

func (f *svcFixture) channel(t *testing.T, kind string, vision bool) (*media, uuid.UUID) {
	t.Helper()
	conn := uuid.New()
	f.store.Kinds = map[uuid.UUID]string{f.thread: kind}
	f.store.Connections = map[uuid.UUID]uuid.UUID{f.thread: conn}
	m := f.providers.Models[f.provider]["gpt-4o"]
	m.Vision = vision
	f.providers.Models[f.provider]["gpt-4o"] = m
	md := &media{data: jpeg}
	f.svc.WithMedia(md)
	return md, conn
}

func (f *svcFixture) post(t *testing.T, m run.Message) {
	t.Helper()
	_, err := f.store.AppendMessage(f.ctx, f.thread, m)
	require.NoError(t, err)
}

func inputParts(t *testing.T, req runtime.TurnRequest) []part {
	t.Helper()
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(req.Input.Message, &body))
	var parts []part
	require.NoError(t, json.Unmarshal(body.Content, &parts), "content is a part list: %s", body.Content)
	return parts
}

func images(parts []part) []part {
	var out []part
	for _, p := range parts {
		if p.Type == "image" {
			out = append(out, p)
		}
	}
	return out
}

func texts(parts []part) string {
	var out []string
	for _, p := range parts {
		if p.Type == "text" {
			out = append(out, p.Text)
		}
	}
	return strings.Join(out, "|")
}

func TestTurn_ImagesFetchedOnlyForThisTurn(t *testing.T) {
	f := newSvc(t)
	md, conn := f.channel(t, "dm", true)
	f.post(t, photoMsg("struk lama", "wamid-old", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	md.fetched = nil

	f.post(t, photoMsg("struk baru", "wamid-new", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.Equal(t, []string{"wamid-new"}, md.ids(), "only this turn's photo is fetched")
	assert.Equal(t, conn, md.fetched[0].conn)
	assert.Equal(t, int64(5<<20), md.fetched[0].maxBytes)

	req := f.rt.LastRequest()
	parts := inputParts(t, req)
	imgs := images(parts)
	require.Len(t, imgs, 1)
	assert.Equal(t, "data", imgs[0].Source.Type)
	assert.Equal(t, "image/jpeg", imgs[0].Source.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(jpeg), imgs[0].Source.Value)
	assert.Nil(t, imgs[0].ID, "the storage-only id never reaches the runtime")
	assert.NotContains(t, string(req.Input.Message), "wamid-new")
	assert.Equal(t, "struk baru", texts(parts))
}

func TestTurn_HistoryPhotoBecomesNote(t *testing.T) {
	f := newSvc(t)
	f.channel(t, "dm", true)
	f.post(t, photoMsg("struk", "wamid-old", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	require.NoError(t, f.snaps.Delete(f.ctx, f.thread))

	f.say(t, "berapa totalnya?")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	require.Nil(t, req.Snapshot)
	history := string(mustMarshal(t, req.History))
	assert.NotContains(t, history, `"image"`, "no image part in rebuilt history")
	assert.NotContains(t, history, "wamid-old")
	assert.Contains(t, history, "[photo]")
	assert.Contains(t, history, "struk")
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestTurn_FourthImageNoted(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	for _, id := range []string{"wamid-1", "wamid-2", "wamid-3", "wamid-4"} {
		f.post(t, photoMsg("", id, nil))
	}
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, []string{"wamid-1", "wamid-2", "wamid-3"}, md.ids(), "the fourth is never fetched")
	parts := inputParts(t, f.rt.LastRequest())
	assert.Len(t, images(parts), 3)
	assert.Equal(t, "[photo not read]", texts(parts))
}

func TestTurn_FetchFailureNotedTurnContinues(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	md.err = errors.New("openwa down")
	f.post(t, photoMsg("struk", "wamid-1", nil))
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, "struk\n[photo not read]", inputContent(t, f.rt.LastRequest()), "with no photo left the input is text")
}

func TestTurn_OversizeImageNoted(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	md.data = append(jpeg, make([]byte, 5<<20)...)
	f.post(t, photoMsg("", "wamid-1", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, "[photo not read]", inputContent(t, f.rt.LastRequest()))
}

func TestTurn_NonVisionModelSkipsPhotos(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", false)
	f.post(t, photoMsg("kopi 2 gelas", "wamid-1", nil))
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status, "the caption still runs")
	assert.Empty(t, md.ids(), "a model that can't see gets no fetch")
	assert.Equal(t, "kopi 2 gelas\n[photo not read]", inputContent(t, f.rt.LastRequest()))
}

func TestTurn_GroupPhotoIsAPartNotJSON(t *testing.T) {
	f := newSvc(t)
	f.channel(t, "group", true)
	budi := uuid.New()
	f.svc.WithNames(names{budi: "Budi"})
	f.sayAs(t, "ini struknya", budi)
	f.post(t, photoMsg("makan siang", "wamid-1", &budi))
	f.post(t, photoMsg("", "wamid-2", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	req := f.rt.LastRequest()
	parts := inputParts(t, req)
	assert.Equal(t, "[Budi] ini struknya|[Budi] makan siang|[member]", texts(parts))
	assert.Len(t, images(parts), 2)
	assert.NotContains(t, string(req.Input.Message), `\"type\"`, "no part is sent as JSON text")
}

func TestTurn_ReadReceiptBestEffort(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	md.readErr = errors.New("openwa down")
	f.say(t, "halo")
	f.post(t, photoMsg("", "wamid-2", nil))
	m := userMsg("lagi", "wamid-3")
	f.post(t, m)
	r := f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, run.StatusCompleted, r.Status, "a failed receipt never stops the turn")
	assert.Equal(t, []string{"wamid-3"}, md.read, "the last inbound message is marked read")
}

func TestTurn_APIThreadSendsNoReceipt(t *testing.T) {
	f := newSvc(t)
	md := &media{data: jpeg}
	f.svc.WithMedia(md)
	f.post(t, userMsg("halo", run.IdempotencyPrefix+"k1"))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Empty(t, md.read)
}

func TestTurn_DetachedThreadNotesPhotos(t *testing.T) {
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	f.store.Connections = nil
	f.post(t, photoMsg("struk", "wamid-1", nil))
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Empty(t, md.ids())
	assert.Empty(t, md.read)
	assert.Equal(t, "struk\n[photo not read]", inputContent(t, f.rt.LastRequest()))
}

func voiceMsg(ext string) run.Message {
	body, _ := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": "[voice note]", "channel": map[string]any{"type": "audio", "voice": true}})
	return run.Message{Role: run.RoleUser, Body: body, ExternalMessageID: ext}
}

func TestTurn_VoiceOnlyTurnSkipsTheModel(t *testing.T) {
	for _, kind := range []string{"dm", "group"} {
		f := newSvc(t)
		md, _ := f.channel(t, kind, true)
		f.post(t, voiceMsg("wamid-1"))
		r := f.execute(t)
		assert.Equal(t, run.StatusCompleted, r.Status, kind)
		assert.Empty(t, f.rt.Requests, "%s: no runtime call", kind)
		assert.Equal(t, run.Usage{}, r.Usage, kind)
		assert.Equal(t, []string{"wamid-1"}, md.read, "%s: still marked read", kind)
	}
	f := newSvc(t)
	f.channel(t, "dm", true)
	f.post(t, voiceMsg("wamid-1"))
	f.say(t, "catat kopi")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, "[voice note]\n\ncatat kopi", inputContent(t, f.rt.LastRequest()), "a mixed turn runs the model")
}
