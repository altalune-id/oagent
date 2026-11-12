package channel_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/run"
)

type prompts struct {
	mu      sync.Mutex
	prompt  *channel.Prompt
	sentIDs map[uuid.UUID]string
}

func (p *prompts) Prompt(context.Context, uuid.UUID) (*channel.Prompt, error) { return p.prompt, nil }

func (p *prompts) PromptSent(_ context.Context, id uuid.UUID, externalID string, _ int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sentIDs == nil {
		p.sentIDs = map[uuid.UUID]string{}
	}
	if _, ok := p.sentIDs[id]; !ok {
		p.sentIDs[id] = externalID
	}
	return nil
}

func promptWorld(t *testing.T, kind string) (*replyWorld, *prompts) {
	t.Helper()
	w := newReplyWorld(t, kind, "the model's text is never sent for a prompt")
	w.store.jobs[w.run].ReplyKind = run.ReplyKindPrompt
	w.conns.conn.ApprovalPrompt = "{preview}\n\nBalas YA atau TIDAK"
	p := &prompts{prompt: &channel.Prompt{ApprovalID: uuid.New(), PublicID: "apr_pub", Preview: "Record expense\n\\*amount\\*: 25000"}}
	w.replier.WithPrompts(p)
	return w, p
}

func TestPrompt_IdempotencyKey(t *testing.T) {
	w, p := promptWorld(t, "dm")
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	sent := w.provider.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, channel.Reply{To: phone, Text: "Record expense\n\\*amount\\*: 25000\n\nBalas YA atau TIDAK", IdempotencyKey: "approval:apr_pub"}, sent[0],
		"the connection's prompt with the preview, keyed by the approval")
	assert.Equal(t, run.ReplySent, w.store.state(w.run))
	assert.Equal(t, "out-1", p.sentIDs[p.prompt.ApprovalID], "the send result is stored for quoted replies")

	w.store.jobs[w.run].State = run.ReplyDue
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Len(t, w.provider.Sent(), 1, "a retried prompt is a no-op at the provider")
}

func TestPrompt_GroupQuotesTheRequester(t *testing.T) {
	w, _ := promptWorld(t, "group")
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	sent := w.provider.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "wamid-9", sent[0].ReplyTo)
}

func TestPrompt_NoPendingApprovalSendsNothing(t *testing.T) {
	w, p := promptWorld(t, "dm")
	p.prompt = nil
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	assert.Empty(t, w.provider.Sent(), "a decided or expired approval has no prompt to send")
	assert.NotEqual(t, run.ReplyDue, w.store.state(w.run))
}

func TestPrompt_StoredTextWithoutPreviewStillShowsIt(t *testing.T) {
	w, _ := promptWorld(t, "dm")
	w.conns.conn.ApprovalPrompt = "Balas YA atau TIDAK"
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	sent := w.provider.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Record expense\n\\*amount\\*: 25000\n\nBalas YA atau TIDAK", sent[0].Text, "a person never approves blind")
}

func TestReply_ExpiredSendsTheConnectionsText(t *testing.T) {
	w := newReplyWorld(t, "dm", "the model's text is never sent for an expired run")
	w.store.jobs[w.run].ReplyKind = run.ReplyKindExpired
	w.conns.conn.ApprovalExpired = "Permintaan ini kedaluwarsa."
	require.NoError(t, w.replier.Send(t.Context(), w.run))
	sent := w.provider.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, channel.Reply{To: phone, Text: "Permintaan ini kedaluwarsa.", IdempotencyKey: "run:runpub:0"}, sent[0])
	assert.Equal(t, run.ReplySent, w.store.state(w.run))
}
