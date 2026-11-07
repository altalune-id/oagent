// Package channeltest is the contract every channel provider passes, plus a fake provider for tests.
package channeltest

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
)

// Harness is what the contract needs from one provider under test.
type Harness struct {
	Provider channel.Provider
	Conn     channel.Connection
	Secrets  channel.Secrets
	// Signed builds a delivery carrying body that Verify must accept with Secrets.
	Signed func(body []byte) *http.Request
	// Inbound is a delivery body Parse turns into Want.
	Inbound []byte
	Want    channel.Inbound
	// Ignored is a delivery body Parse refuses as an IgnoredError.
	Ignored []byte
	// Sent reports what reached the remote side, in order.
	Sent func() []channel.Reply
	// Media is served by FetchMedia as MediaBody with MediaMime.
	Media     channel.MediaRef
	MediaBody []byte
	MediaMime string
	// Read reports the message ids marked read, in order.
	Read func() []string
}

// Run checks the provider newHarness builds against the channel contract.
func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("Verify accepts a signed delivery and nothing else", func(t *testing.T) {
		h := newHarness(t)
		body := h.Inbound
		require.NoError(t, h.Provider.Verify(h.Signed(body), body, h.Conn, h.Secrets))
		assert.NotEmpty(t, h.Provider.DeliveryID(h.Signed(body)), "a signed delivery names its delivery id")
		tampered := append(bytes.Clone(body), ' ')
		assert.Error(t, h.Provider.Verify(h.Signed(body), tampered, h.Conn, h.Secrets), "a changed body")
		assert.Error(t, h.Provider.Verify(h.Signed(body), body, h.Conn, channel.Secrets{Signing: []string{"not-the-secret"}}), "another secret")
		assert.Error(t, h.Provider.Verify(h.Signed(body), body, h.Conn, channel.Secrets{}), "no secret at all")
	})
	t.Run("Parse turns a delivery into a provider-neutral message", func(t *testing.T) {
		h := newHarness(t)
		got, err := h.Provider.Parse(h.Inbound)
		require.NoError(t, err)
		assert.Equal(t, h.Want, got)
		_, err = h.Provider.Parse(h.Ignored)
		assert.True(t, channel.IsIgnoredError(err), "a delivery with no work: %v", err)
		_, err = h.Provider.Parse([]byte("not json"))
		require.Error(t, err)
		assert.False(t, channel.IsIgnoredError(err), "garbage is an error, not a quiet success")
	})
	t.Run("Send delivers once per idempotency key", func(t *testing.T) {
		h := newHarness(t)
		r := channel.Reply{To: "addr-1", Text: "hello", IdempotencyKey: "run:R1:1"}
		id, err := h.Provider.Send(t.Context(), h.Conn, h.Secrets, r)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		again, err := h.Provider.Send(t.Context(), h.Conn, h.Secrets, r)
		require.NoError(t, err)
		assert.Equal(t, id, again, "a retried part answers like the first")
		second, err := h.Provider.Send(t.Context(), h.Conn, h.Secrets, channel.Reply{To: "addr-1", Text: "part two", IdempotencyKey: "run:R1:2"})
		require.NoError(t, err)
		assert.NotEqual(t, id, second)
		sent := h.Sent()
		require.Len(t, sent, 2, "each part once")
		assert.Equal(t, "hello", sent[0].Text)
		assert.Equal(t, "part two", sent[1].Text)
	})
	t.Run("FetchMedia is bounded by the caller", func(t *testing.T) {
		h := newHarness(t)
		body, mime, err := h.Provider.FetchMedia(t.Context(), h.Conn, h.Secrets, h.Media, int64(len(h.MediaBody)))
		require.NoError(t, err)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		assert.Equal(t, h.MediaBody, got)
		assert.Equal(t, h.MediaMime, mime)
		over, _, err := h.Provider.FetchMedia(t.Context(), h.Conn, h.Secrets, h.Media, int64(len(h.MediaBody))-1)
		if err == nil {
			_, err = io.ReadAll(over)
			_ = over.Close()
		}
		assert.True(t, channel.IsTooLargeError(err), "media over the limit: %v", err)
	})
	t.Run("MarkRead names the inbound message", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.Provider.MarkRead(t.Context(), h.Conn, h.Secrets, h.Want.ExternalID))
		assert.Equal(t, []string{h.Want.ExternalID}, h.Read())
	})
}
