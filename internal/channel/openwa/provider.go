package openwa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/webhook"
)

const (
	eventMatched      = "message.matched"
	headerTimestamp   = "X-Openwa-Timestamp"
	headerSignature   = "X-Openwa-Signature"
	headerDeliveryID  = "X-Openwa-Delivery-Id"
	signatureWindow   = 5 * time.Minute
	sendTimeout       = 15 * time.Second
	mediaTimeout      = 15 * time.Second
	readTimeout       = 5 * time.Second
	maxResponseBytes  = 64 << 10
	maxErrorBodyBytes = 512
)

var messageIDPattern = regexp.MustCompile(`^msg_[A-Za-z0-9_-]{16}$`)

// Provider talks to one OpenWA deployment per connection through SSRF-guarded clients.
type Provider struct {
	send, media, read *http.Client
}

var _ channel.Provider = (*Provider)(nil)

// New returns the OpenWA provider; allowPrivateHosts lets it reach OpenWA on a private network.
func New(allowPrivateHosts bool) *Provider {
	client := func(timeout time.Duration, limit int64) *http.Client {
		return httpclient.New(httpclient.WithAllowPrivateHosts(allowPrivateHosts), httpclient.WithoutProxy(),
			httpclient.WithTimeout(timeout), httpclient.WithResponseBodyLimit(limit))
	}
	return &Provider{send: client(sendTimeout, maxResponseBytes), media: client(mediaTimeout, 0), read: client(readTimeout, maxResponseBytes)}
}

// Verify checks the delivery's v1 signature against every stored signing secret within five minutes.
func (p *Provider) Verify(r *http.Request, body []byte, _ channel.Connection, secrets channel.Secrets) error {
	return webhook.VerifyV1(secrets.Signing, r.Header.Get(headerTimestamp), r.Header.Get(headerSignature), body, time.Now(), signatureWindow)
}

// DeliveryID is OpenWA's id for one event to one endpoint, the same on every retry.
func (p *Provider) DeliveryID(r *http.Request) string { return r.Header.Get(headerDeliveryID) }

// Parse turns a message.matched envelope into a provider-neutral message; other events and message types are ignored.
func (p *Provider) Parse(body []byte) (channel.Inbound, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return channel.Inbound{}, fmt.Errorf("openwa: parse envelope: %w", err)
	}
	if env.Type != eventMatched {
		return channel.Inbound{}, &channel.IgnoredError{Reason: "event type"}
	}
	var ev messageEvent
	if err := json.Unmarshal(env.Data, &ev); err != nil {
		return channel.Inbound{}, fmt.Errorf("openwa: parse message: %w", err)
	}
	if ev.Sender.FromMe {
		return channel.Inbound{}, &channel.IgnoredError{Reason: "from me"}
	}
	m := ev.Message
	switch m.Type {
	case "text", "image", "audio", "video", "document", "location", "contact":
	default:
		return channel.Inbound{}, &channel.IgnoredError{Reason: "message type"}
	}
	in := channel.Inbound{
		EventType: env.Type, DeviceID: ev.Device.ID, ExternalID: m.ID, ChatKey: ev.Chat.ID, Kind: channel.Kind(ev.Chat.Kind),
		Sender: channel.Sender{Phone: digits(ev.Sender.Phone), LID: digits(userPart(ev.Sender.LID)), DisplayName: ev.Sender.PushName},
		Type:   m.Type, Text: textOf(m), Time: m.Timestamp.UTC(),
	}
	switch in.Kind {
	case channel.KindGroup:
		in.ChatAddress = ev.Chat.JID
	case channel.KindDM:
	default:
		return channel.Inbound{}, fmt.Errorf("openwa: chat kind %q", ev.Chat.Kind)
	}
	if m.Quoted != nil {
		in.QuotedID = m.Quoted.WAID
	}
	if m.Media != nil {
		in.Media = &channel.MediaRef{MessageID: m.ID, Mime: baseMime(m.Media.Mime), Size: m.Media.Size, Voice: m.Media.Voice}
	}
	in.Text, in.Mentions = mentions(in.Text, m.Mentions, digits(ev.Device.Phone))
	return in, nil
}

// Send posts one reply as the connection's device; a repeated idempotency key answers like the first.
func (p *Provider) Send(ctx context.Context, conn channel.Connection, secrets channel.Secrets, r channel.Reply) (string, error) {
	payload, err := json.Marshal(struct {
		To      string `json:"to"`
		Text    string `json:"text"`
		ReplyTo string `json:"reply_to,omitempty"`
	}{r.To, r.Text, r.ReplyTo})
	if err != nil {
		return "", fmt.Errorf("openwa: encode send: %w", err)
	}
	req, err := newRequest(ctx, http.MethodPost, projectURL(conn)+"/devices/"+url.PathEscape(conn.DeviceID)+"/messages", payload, secrets)
	if err != nil {
		return "", err
	}
	req.Header.Set("Idempotency-Key", r.IdempotencyKey)
	resp, err := do(p.send, req, "send")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		return "", unavailable("send", errors.New("no message id in the response"))
	}
	return out.ID, nil
}

// FetchMedia reads an inbound JPEG, PNG, WebP or Ogg file of at most maxBytes, from a URL built from the connection and message id only.
func (p *Provider) FetchMedia(
	ctx context.Context, conn channel.Connection, secrets channel.Secrets, ref channel.MediaRef, maxBytes int64,
) (io.ReadCloser, string, error) {
	if !messageIDPattern.MatchString(ref.MessageID) {
		return nil, "", &MessageIDError{}
	}
	if ref.Size > maxBytes {
		return nil, "", &channel.TooLargeError{Limit: maxBytes}
	}
	req, err := newRequest(ctx, http.MethodGet, projectURL(conn)+"/messages/"+ref.MessageID+"/media", nil, secrets)
	if err != nil {
		return nil, "", err
	}
	resp, err := do(p.media, req, "media")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	mimeType := baseMime(resp.Header.Get("Content-Type"))
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp", "audio/ogg":
	default:
		return nil, "", &channel.MediaTypeError{}
	}
	if resp.ContentLength > maxBytes {
		return nil, "", &channel.TooLargeError{Limit: maxBytes}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", unavailable("media", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", &channel.TooLargeError{Limit: maxBytes}
	}
	return io.NopCloser(bytes.NewReader(data)), mimeType, nil
}

// MarkRead marks the chat of an inbound message read up to that message.
func (p *Provider) MarkRead(ctx context.Context, conn channel.Connection, secrets channel.Secrets, externalID string) error {
	if !messageIDPattern.MatchString(externalID) {
		return &MessageIDError{}
	}
	req, err := newRequest(ctx, http.MethodPost, projectURL(conn)+"/messages/"+externalID+"/read", nil, secrets)
	if err != nil {
		return err
	}
	resp, err := do(p.read, req, "read")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func projectURL(conn channel.Connection) string {
	return strings.TrimRight(conn.BaseURL, "/") +
		"/api/v1/orgs/" + url.PathEscape(conn.OpenWAOrg) + "/projects/" + url.PathEscape(conn.OpenWAProject)
}

func newRequest(ctx context.Context, method, target string, body []byte, secrets channel.Secrets) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, unavailable("request", errors.New("invalid OpenWA URL"))
	}
	req.Header.Set("Authorization", "Bearer "+secrets.DeviceKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// SECURITY: errors carry the operation and status only; never the URL, its query, the key or the response body.
func do(client *http.Client, req *http.Request, op string) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return nil, unavailable(op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &channel.KeyRefusedError{}
		}
		return nil, unavailable(op, fmt.Errorf("status %d", resp.StatusCode))
	}
	return resp, nil
}

func unavailable(op string, err error) error {
	return &channel.ProviderUnavailableError{Cause: fmt.Errorf("openwa: %s: %w", op, err)}
}

func textOf(m message) string {
	switch m.Type {
	case "location":
		if m.Location == nil {
			return ""
		}
		if m.Location.Name != "" {
			return m.Location.Name
		}
		return m.Location.Address
	default:
		return strings.TrimSpace(strings.Join(nonEmpty(m.Body, m.Caption), "\n"))
	}
}

func nonEmpty(values ...string) []string {
	out := values[:0]
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// SECURITY: only digit user parts leave Parse, so a mention can be resolved without keeping the JID.
func mentions(text string, jids []string, self string) (stripped string, users []string) {
	out := make([]string, 0, len(jids))
	for _, jid := range jids {
		user := digits(userPart(jid))
		if user == "" {
			continue
		}
		if user == self {
			text = strings.TrimSpace(regexp.MustCompile(`@`+user+`\b\s*`).ReplaceAllString(text, ""))
			continue
		}
		out = append(out, user)
	}
	if len(out) == 0 {
		return text, nil
	}
	return text, out
}

func userPart(jid string) string {
	user, _, _ := strings.Cut(jid, "@")
	user, _, _ = strings.Cut(user, ":")
	return user
}

func digits(s string) string {
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return ""
	}
	return s
}

func baseMime(v string) string {
	base, _, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return base
}
