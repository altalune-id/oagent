package openwa_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/channel/channeltest"
	"altalune.id/oagent/internal/channel/openwa"
	"altalune.id/oagent/internal/webhook"
)

const (
	deviceID  = "dev_V1StGXR8Z5jdHi6B"
	deviceKey = "owk_live_device_key_never_logged"
	imageID   = "msg_Im2Cr9Mn4Pq7St0e"
)

func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "message_matched_"+name+".json"))
	require.NoError(t, err)
	return raw
}

func envelope(t *testing.T, eventType string, data json.RawMessage) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{
		"id": "evt_0193a2b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b", "type": eventType, "api_version": "v1", "created_at": "2026-09-28T10:15:01Z",
		"tenant": map[string]any{"org_id": "0193a2b4-0000-7000-8000-000000000001", "project_id": "0193a2b4-0000-7000-8000-000000000002", "project_slug": "main"},
		"data":   data,
	})
	require.NoError(t, err)
	return out
}

type sentRequest struct {
	Path, Auth, IdempotencyKey, ContentType string
	Body                                    map[string]any
}

type fakeOpenWA struct {
	mu        sync.Mutex
	srv       *httptest.Server
	sent      []sentRequest
	ids       map[string]string
	read      []string
	media     []byte
	mediaMime string
	status    int
}

func newFakeOpenWA(t *testing.T, prefix string) *fakeOpenWA {
	t.Helper()
	f := &fakeOpenWA{ids: map[string]string{}, media: []byte("\xff\xd8\xff fake jpeg"), mediaMime: "image/jpeg"}
	base := prefix + "/api/v1/orgs/acme/projects/main"
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+base+"/devices/{device}/messages", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.status != 0 {
			http.Error(w, `{"code":"down","detail":"`+r.Header.Get("Authorization")+`"}`, f.status)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		key := r.Header.Get("Idempotency-Key")
		id, seen := f.ids[key]
		if !seen {
			id = "msg_Out" + strconv.Itoa(len(f.ids)) + "xxxxxxxxxxxx"
			f.ids[key] = id
			f.sent = append(f.sent, sentRequest{Path: r.URL.Path, Auth: r.Header.Get("Authorization"), IdempotencyKey: key, ContentType: r.Header.Get("Content-Type"), Body: body})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "device_id": r.PathValue("device")})
	})
	mux.HandleFunc("GET "+base+"/messages/{message}/media", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		status := f.status
		f.mu.Unlock()
		if status != 0 {
			http.Error(w, "down", status)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+deviceKey {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", f.mediaMime)
		_, _ = w.Write(f.media)
	})
	mux.HandleFunc("POST "+base+"/messages/{message}/read", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.status != 0 {
			http.Error(w, "down", f.status)
			return
		}
		f.read = append(f.read, r.PathValue("message"))
		w.WriteHeader(http.StatusNoContent)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenWA) replies() []channel.Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]channel.Reply, 0, len(f.sent))
	for _, s := range f.sent {
		replyTo, _ := s.Body["reply_to"].(string)
		out = append(out, channel.Reply{To: s.Body["to"].(string), Text: s.Body["text"].(string), ReplyTo: replyTo, IdempotencyKey: s.IdempotencyKey})
	}
	return out
}

func (f *fakeOpenWA) marked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.read...)
}

func conn(base string) channel.Connection {
	return channel.Connection{PublicID: "cnx_test", Provider: channel.ProviderOpenWA, BaseURL: base, OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: deviceID}
}

func secrets() channel.Secrets {
	return channel.Secrets{Signing: []string{"whsec_primary", "whsec_secondary"}, DeviceKey: deviceKey}
}

func signed(body []byte, secret string, at time.Time) *http.Request {
	ts := strconv.FormatInt(at.Unix(), 10)
	r := httptest.NewRequest(http.MethodPost, "/hooks/openwa/cnx_test", bytes.NewReader(body))
	r.Header.Set("X-Openwa-Event-Type", "message.matched")
	r.Header.Set("X-Openwa-Timestamp", ts)
	r.Header.Set("X-Openwa-Signature", webhook.Sign(secret, ts, body))
	r.Header.Set("X-Openwa-Delivery-Id", "dlv_0193a2b4")
	return r
}

func TestProvider_PassesTheContract(t *testing.T) {
	channeltest.Run(t, func(t *testing.T) channeltest.Harness {
		f := newFakeOpenWA(t, "")
		textBody := envelope(t, "message.matched", fixture(t, "text"))
		return channeltest.Harness{
			Provider: openwa.New(true), Conn: conn(f.srv.URL), Secrets: secrets(),
			Signed:  func(body []byte) *http.Request { return signed(body, "whsec_secondary", time.Now()) },
			Inbound: textBody,
			Want: channel.Inbound{
				EventType: "message.matched", DeviceID: deviceID, ExternalID: "msg_Tx1Bq8Lm3Np6Rs9d", ChatKey: "cht_Dm4Kp7Wx2Rt9Ya1c", Kind: channel.KindDM,
				Sender: channel.Sender{Phone: "628111222333", LID: "12345678901234", DisplayName: "Budi"}, Type: "text", Text: "Halo, stok kopi susu masih ada?",
				Time: time.Date(2026, 9, 28, 10, 15, 0, 0, time.UTC),
			},
			Ignored: envelope(t, "message.matched", fixture(t, "reaction")),
			Sent:    f.replies,
			Media:   channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: int64(len(f.media))}, MediaBody: f.media, MediaMime: "image/jpeg",
			Read: f.marked,
		}
	})
}

func TestVerify_WindowAndHeaders(t *testing.T) {
	p := openwa.New(true)
	body := envelope(t, "message.matched", fixture(t, "text"))
	c, s := conn("https://wa.example.com"), secrets()
	require.NoError(t, p.Verify(signed(body, "whsec_primary", time.Now().Add(-4*time.Minute)), body, c, s))
	assert.Error(t, p.Verify(signed(body, "whsec_primary", time.Now().Add(-6*time.Minute)), body, c, s), "outside the five-minute window")
	r := signed(body, "whsec_primary", time.Now())
	r.Header.Del("X-Openwa-Timestamp")
	assert.Error(t, p.Verify(r, body, c, s), "no timestamp")
}

func TestParse_Fixtures(t *testing.T) {
	p := openwa.New(true)
	parse := func(name string) channel.Inbound {
		t.Helper()
		in, err := p.Parse(envelope(t, "message.matched", fixture(t, name)))
		require.NoError(t, err, name)
		return in
	}
	image := parse("image")
	assert.Equal(t, "foto struk kemarin", image.Text, "the caption is the text")
	assert.Equal(t, &channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: 48213}, image.Media)

	voice := parse("voice")
	assert.Equal(t, "audio", voice.Type)
	assert.Equal(t, &channel.MediaRef{MessageID: "msg_Vc3Ds0No5Qr8Tu1f", Mime: "audio/ogg", Size: 9120, Voice: true}, voice.Media, "mime parameters dropped")

	group := parse("group_mention")
	assert.Equal(t, channel.KindGroup, group.Kind)
	assert.Equal(t, "120363012345678901@g.us", group.ChatAddress, "a group keeps its JID as the send target")
	assert.Equal(t, "tolong cek pesanan Sari", group.Text, "the agent's own mention is stripped")
	assert.Empty(t, group.Mentions, "the agent's own mention is not returned")
	assert.Equal(t, "3EB00199C1F000007000800000000000E000", group.QuotedID)
	assert.Equal(t, channel.Sender{Phone: "628444555666", LID: "98765432109876", DisplayName: "Sari"}, group.Sender)

	hidden := parse("hidden_phone")
	assert.Empty(t, hidden.Sender.Phone)
	assert.Equal(t, "55667788990011", hidden.Sender.LID)
	assert.Empty(t, hidden.ChatAddress)

	assert.Equal(t, "video gudang", parse("video").Text)
	assert.Equal(t, "video/mp4", parse("video").Media.Mime)
	assert.Equal(t, "application/pdf", parse("document").Media.Mime)
	assert.Equal(t, "Toko Kopi Budi", parse("location").Text, "a location is named, not given as coordinates")
	assert.Equal(t, "Sari Kurir", parse("contact").Text)

	_, err := p.Parse(envelope(t, "message.matched", fixture(t, "reaction")))
	assert.True(t, channel.IsIgnoredError(err), "reactions carry no work")
	_, err = p.Parse(envelope(t, "message.received", fixture(t, "text")))
	assert.True(t, channel.IsIgnoredError(err), "only matched messages carry work")
	fromMe := strings.Replace(string(fixture(t, "text")), `"from_me": false`, `"from_me": true`, 1)
	_, err = p.Parse(envelope(t, "message.matched", json.RawMessage(fromMe)))
	assert.True(t, channel.IsIgnoredError(err), "the device's own messages carry no work")
	badKind := strings.Replace(string(fixture(t, "text")), `"kind": "dm"`, `"kind": "broadcast"`, 1)
	_, err = p.Parse(envelope(t, "message.matched", json.RawMessage(badKind)))
	require.Error(t, err)
	assert.False(t, channel.IsIgnoredError(err))
}

func TestParse_MentionsAreDigitsAndOnlyTheAgentIsStripped(t *testing.T) {
	data := strings.NewReplacer(
		`"@628123456789 tolong cek pesanan Sari"`, `"@628123456789 tanya @6281234567890 dan @98765432109876"`,
		`"628123456789"`+"\n    ]", `"628123456789", "6281234567890@s.whatsapp.net", "98765432109876@lid"`+"\n    ]",
	).Replace(string(fixture(t, "group_mention")))
	in, err := openwa.New(true).Parse(envelope(t, "message.matched", json.RawMessage(data)))
	require.NoError(t, err)
	assert.Equal(t, "tanya @6281234567890 dan @98765432109876", in.Text, "a longer number that starts with the agent's is kept")
	assert.Equal(t, []string{"6281234567890", "98765432109876"}, in.Mentions)
}

func TestParse_DMJIDNotKept(t *testing.T) {
	p := openwa.New(true)
	for _, name := range []string{"text", "image", "voice", "hidden_phone"} {
		in, err := p.Parse(envelope(t, "message.matched", fixture(t, name)))
		require.NoError(t, err)
		assert.Empty(t, in.ChatAddress, "%s: a DM JID holds the phone, so it is never kept", name)
	}
}

func TestFetchMedia_URLBuiltNotFromPayload(t *testing.T) {
	f := newFakeOpenWA(t, "/openwa")
	elsewhere := 0
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { elsewhere++ }))
	t.Cleanup(attacker.Close)
	p := openwa.New(true)
	data := strings.ReplaceAll(string(fixture(t, "image")), "https://wa.example.com/api/v1/orgs/acme/projects/main/messages/"+imageID+"/media", attacker.URL+"/steal")
	in, err := p.Parse(envelope(t, "message.matched", json.RawMessage(data)))
	require.NoError(t, err)
	body, mime, err := p.FetchMedia(t.Context(), conn(f.srv.URL+"/openwa/"), secrets(), *in.Media, 1<<20)
	require.NoError(t, err, "the base URL path prefix is kept")
	got, _ := io.ReadAll(body)
	_ = body.Close()
	assert.Equal(t, f.media, got)
	assert.Equal(t, "image/jpeg", mime)
	assert.Zero(t, elsewhere, "the payload's media.url is never fetched")
}

func TestFetchMedia_BadMessageIDRefused(t *testing.T) {
	f := newFakeOpenWA(t, "")
	p := openwa.New(true)
	for _, id := range []string{"", "msg_short", "../../devices", "msg_Im2Cr9Mn4Pq7St0e/../x", "dev_V1StGXR8Z5jdHi6B", "msg_Im2Cr9Mn4Pq7St0e?x=1"} {
		_, _, err := p.FetchMedia(t.Context(), conn(f.srv.URL), secrets(), channel.MediaRef{MessageID: id, Mime: "image/jpeg", Size: 10}, 1<<20)
		assert.Error(t, err, "%q", id)
		assert.Error(t, p.MarkRead(t.Context(), conn(f.srv.URL), secrets(), id), "%q", id)
	}
	assert.Empty(t, f.marked())
}

func TestFetchMedia_CapAndType(t *testing.T) {
	f := newFakeOpenWA(t, "")
	p := openwa.New(true)
	ref := channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: int64(len(f.media))}
	_, _, err := p.FetchMedia(t.Context(), conn(f.srv.URL), secrets(), channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: 6 << 20}, 5<<20)
	assert.True(t, channel.IsTooLargeError(err), "a declared size over the cap is refused before fetching: %v", err)

	f.media = bytes.Repeat([]byte("x"), 100)
	_, _, err = p.FetchMedia(t.Context(), conn(f.srv.URL), secrets(), ref, 50)
	assert.True(t, channel.IsTooLargeError(err), "the bytes read are capped whatever the payload declared: %v", err)

	for _, m := range []string{"image/gif", "image/svg+xml", "text/html", "application/octet-stream"} {
		f.mediaMime = m
		_, _, err = p.FetchMedia(t.Context(), conn(f.srv.URL), secrets(), ref, 1<<20)
		assert.Error(t, err, m)
	}
	f.mediaMime = "image/webp; charset=binary"
	_, mime, err := p.FetchMedia(t.Context(), conn(f.srv.URL), secrets(), ref, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, "image/webp", mime)

	_, _, err = p.FetchMedia(t.Context(), conn(f.srv.URL), channel.Secrets{DeviceKey: "wrong"}, ref, 1<<20)
	assert.True(t, channel.IsProviderUnavailableError(err), "%v", err)
}

func TestSend_RequestShape(t *testing.T) {
	f := newFakeOpenWA(t, "/openwa")
	p := openwa.New(true)
	id, err := p.Send(t.Context(), conn(f.srv.URL+"/openwa"), secrets(),
		channel.Reply{To: "120363012345678901@g.us", Text: "Sudah dicek", ReplyTo: "msg_Gm4Et1Op6Rs9Uv2g", IdempotencyKey: "run:run_abc:1"})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	_, err = p.Send(t.Context(), conn(f.srv.URL+"/openwa"), secrets(), channel.Reply{To: "628111222333@s.whatsapp.net", Text: "Halo", IdempotencyKey: "run:run_abc:2"})
	require.NoError(t, err)
	require.Len(t, f.sent, 2)
	first := f.sent[0]
	assert.Equal(t, "/openwa/api/v1/orgs/acme/projects/main/devices/"+deviceID+"/messages", first.Path)
	assert.Equal(t, "Bearer "+deviceKey, first.Auth)
	assert.Equal(t, "run:run_abc:1", first.IdempotencyKey)
	assert.Equal(t, "application/json", first.ContentType)
	assert.Equal(t, map[string]any{"to": "120363012345678901@g.us", "text": "Sudah dicek", "reply_to": "msg_Gm4Et1Op6Rs9Uv2g"}, first.Body)
	assert.Equal(t, map[string]any{"to": "628111222333@s.whatsapp.net", "text": "Halo"}, f.sent[1].Body, "no reply_to unless given")
}

func TestProvider_NoKeyInErrors(t *testing.T) {
	f := newFakeOpenWA(t, "")
	f.status = http.StatusBadGateway
	p := openwa.New(true)
	c := conn(f.srv.URL)
	_, sendErr := p.Send(t.Context(), c, secrets(), channel.Reply{To: "628111222333@s.whatsapp.net", Text: "x", IdempotencyKey: "k"})
	readErr := p.MarkRead(t.Context(), c, secrets(), imageID)
	f.srv.Close()
	_, _, mediaErr := p.FetchMedia(t.Context(), c, secrets(), channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: 1}, 10)
	_, dialErr := p.Send(t.Context(), conn("http://127.0.0.1:1/?token=query_secret"), secrets(), channel.Reply{To: "a", Text: "x", IdempotencyKey: "k"})
	for name, err := range map[string]error{"send": sendErr, "read": readErr, "media": mediaErr, "dial": dialErr} {
		require.Error(t, err, name)
		assert.True(t, channel.IsProviderUnavailableError(err), "%s: %v", name, err)
		assert.NotContains(t, err.Error(), deviceKey, name)
		assert.NotContains(t, err.Error(), "query_secret", name)
		assert.NotContains(t, err.Error(), "/api/v1", "%s: no URL in the error", name)
	}
}

func TestProvider_RefusesPrivateHostsUnlessAllowed(t *testing.T) {
	f := newFakeOpenWA(t, "")
	_, err := openwa.New(false).Send(t.Context(), conn(f.srv.URL), secrets(), channel.Reply{To: "a", Text: "x", IdempotencyKey: "k"})
	assert.True(t, channel.IsProviderUnavailableError(err), "%v", err)
	assert.Empty(t, f.sent)
}

func TestProvider_401IsKeyRefused(t *testing.T) {
	f := newFakeOpenWA(t, "")
	f.status = http.StatusUnauthorized
	p := openwa.New(true)
	c := conn(f.srv.URL)
	_, sendErr := p.Send(t.Context(), c, secrets(), channel.Reply{To: "628111222333@s.whatsapp.net", Text: "x", IdempotencyKey: "k"})
	readErr := p.MarkRead(t.Context(), c, secrets(), imageID)
	_, _, mediaErr := p.FetchMedia(t.Context(), c, secrets(), channel.MediaRef{MessageID: imageID, Mime: "image/jpeg", Size: 1}, 10)
	for name, err := range map[string]error{"send": sendErr, "read": readErr, "media": mediaErr} {
		assert.True(t, channel.IsKeyRefusedError(err), "%s: %v", name, err)
	}
}
