package channeltest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"altalune.id/oagent/internal/channel"
)

// FakeSignatureHeader carries the fake provider's signature: hex HMAC-SHA256 of the body.
const FakeSignatureHeader = "X-Fake-Signature"

// Fake is an in-memory channel provider; it parses a channel.Inbound as JSON and ignores reactions.
type Fake struct {
	mu    sync.Mutex
	sent  []channel.Reply
	ids   map[string]string
	read  []string
	Media map[string][]byte
	// SendErr fails every Send.
	SendErr error
}

var _ channel.Provider = (*Fake)(nil)

// Sign is the fake's signature over body with secret.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify implements channel.Provider.
func (f *Fake) Verify(r *http.Request, body []byte, _ channel.Connection, secrets channel.Secrets) error {
	got := r.Header.Get(FakeSignatureHeader)
	for _, s := range secrets.Signing {
		if s != "" && hmac.Equal([]byte(got), []byte(Sign(s, body))) {
			return nil
		}
	}
	return errors.New("channeltest: bad signature")
}

// FakeDeliveryHeader carries the fake provider's delivery id.
const FakeDeliveryHeader = "X-Fake-Delivery"

// DeliveryID implements channel.Provider.
func (f *Fake) DeliveryID(r *http.Request) string { return r.Header.Get(FakeDeliveryHeader) }

// Parse implements channel.Provider.
func (f *Fake) Parse(body []byte) (channel.Inbound, error) {
	var in channel.Inbound
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return channel.Inbound{}, err
	}
	if in.Type == "reaction" {
		return channel.Inbound{}, &channel.IgnoredError{Reason: "reaction"}
	}
	return in, nil
}

// Send implements channel.Provider.
func (f *Fake) Send(_ context.Context, _ channel.Connection, _ channel.Secrets, r channel.Reply) (string, error) {
	if f.SendErr != nil {
		return "", f.SendErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ids == nil {
		f.ids = map[string]string{}
	}
	if id, ok := f.ids[r.IdempotencyKey]; ok && r.IdempotencyKey != "" {
		return id, nil
	}
	f.sent = append(f.sent, r)
	id := "out-" + strconv.Itoa(len(f.sent))
	f.ids[r.IdempotencyKey] = id
	return id, nil
}

// FetchMedia implements channel.Provider.
func (f *Fake) FetchMedia(
	_ context.Context, _ channel.Connection, _ channel.Secrets, ref channel.MediaRef, maxBytes int64,
) (io.ReadCloser, string, error) {
	f.mu.Lock()
	body, ok := f.Media[ref.MessageID]
	f.mu.Unlock()
	if !ok {
		return nil, "", errors.New("channeltest: no media")
	}
	if int64(len(body)) > maxBytes {
		return nil, "", &channel.TooLargeError{Limit: maxBytes}
	}
	return io.NopCloser(bytes.NewReader(body)), ref.Mime, nil
}

// MarkRead implements channel.Provider.
func (f *Fake) MarkRead(_ context.Context, _ channel.Connection, _ channel.Secrets, externalID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, externalID)
	return nil
}

// Sent reports every reply delivered, in order.
func (f *Fake) Sent() []channel.Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]channel.Reply(nil), f.sent...)
}

// Read reports every message id marked read, in order.
func (f *Fake) Read() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.read...)
}

// NewFake builds a contract harness around a Fake.
func NewFake(t *testing.T) Harness {
	t.Helper()
	media := []byte("\x89PNG fake image bytes")
	f := &Fake{Media: map[string][]byte{"msg-img": media}}
	want := channel.Inbound{
		EventType: "message.matched", DeviceID: "dev_V1StGXR8Z5jdHi6B", ExternalID: "msg-1", ChatKey: "chat-1", Kind: channel.KindDM,
		Sender: channel.Sender{Phone: "628111222333", DisplayName: "Budi"}, Type: "text", Text: "hello",
		Time: time.Date(2026, 9, 28, 10, 15, 0, 0, time.UTC),
	}
	inbound, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	ignored, err := json.Marshal(channel.Inbound{Type: "reaction", ExternalID: "msg-2"})
	if err != nil {
		t.Fatal(err)
	}
	secrets := channel.Secrets{Signing: []string{"fake-secret"}, DeviceKey: "fake-key"}
	return Harness{
		Provider: f, Conn: channel.Connection{PublicID: "cnx_fake"}, Secrets: secrets,
		Signed: func(body []byte) *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/hooks/fake/cnx_fake", bytes.NewReader(body))
			r.Header.Set(FakeSignatureHeader, Sign(secrets.Signing[0], body))
			r.Header.Set(FakeDeliveryHeader, "dlv-1")
			return r
		},
		Inbound: inbound, Want: want, Ignored: ignored, Sent: f.Sent,
		Media: channel.MediaRef{MessageID: "msg-img", Mime: "image/png", Size: int64(len(media))}, MediaBody: media, MediaMime: "image/png",
		Read: f.Read,
	}
}
