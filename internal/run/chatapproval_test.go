package run_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type room struct {
	f         *svcFixture
	a         *fakes.RunApprovals
	parked    *run.Run
	requester uuid.UUID
	member    uuid.UUID
	at        time.Time
	n         int
}

// NOTE: newRoom parks a WhatsApp thread of kind on an approval the requester raised, with its prompt sent as wamid-prompt.
func newRoom(t *testing.T, kind string) *room {
	t.Helper()
	c := newParked(t, kind)
	c.sendPrompt()
	return c
}

// NOTE: newParked parks the thread as newRoom does, before its prompt is sent.
func newParked(t *testing.T, kind string) *room {
	t.Helper()
	f := newSvc(t)
	_, conn := f.channel(t, kind, true)
	f.withTexts(conn)
	c := &room{f: f, a: f.withApprovals(), requester: uuid.New(), member: uuid.New(), at: time.Now().Add(-time.Hour)}
	c.post(t, c.requester, "record lunch 25000", "")
	c.parked = f.execute(t, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
	require.Equal(t, run.StatusAwaitingApproval, c.parked.Status)
	return c
}

func (c *room) post(t *testing.T, speaker uuid.UUID, text, quoted string) uuid.UUID {
	t.Helper()
	c.n++
	c.at = c.at.Add(time.Second)
	body, err := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": text, "channel": map[string]any{"type": "text", "quoted": quoted}})
	require.NoError(t, err)
	at := c.at
	c.f.post(t, run.Message{Role: run.RoleUser, Body: body, SpeakerID: &speaker, ExternalMessageID: "wamid-" + string(rune('a'+c.n)), ChannelTime: &at})
	return c.f.store.Messages[len(c.f.store.Messages)-1].ID
}

// NOTE: sendPrompt records the prompt as sent now, as wamid-prompt, after every message stored so far.
func (c *room) sendPrompt() {
	var last int64
	for _, m := range c.f.store.Messages {
		last = max(last, m.Seq)
	}
	c.a.PromptSent(c.parked.ID, "wamid-prompt", last)
}

func (c *room) message(id uuid.UUID) run.Message {
	for _, m := range c.f.store.Messages {
		if m.ID == id {
			return m
		}
	}
	return run.Message{}
}

func (c *room) drain(t *testing.T) *run.Run {
	t.Helper()
	return c.f.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
}

func (c *room) assertParked(t *testing.T, msgs ...uuid.UUID) {
	t.Helper()
	calls := c.f.rt.Calls()
	assert.Nil(t, c.drain(t), "the thread stays parked")
	assert.Equal(t, calls, c.f.rt.Calls(), "no turn reached the runtime")
	assert.Empty(t, c.a.Answers())
	for _, id := range msgs {
		assert.Equal(t, run.MessagePending, c.message(id).Status, "the message waits")
	}
}

func (c *room) assertDecided(t *testing.T, r *run.Run, approved bool, text string, by uuid.UUID, answer uuid.UUID) {
	t.Helper()
	require.NotNil(t, r)
	assert.Equal(t, c.parked.ID, r.ID, "the parked run resumes")
	assert.Equal(t, run.StatusCompleted, r.Status)
	answers := c.a.Answers()
	require.Len(t, answers, 1)
	assert.Equal(t, approved, answers[0].Approved)
	assert.Equal(t, text, answers[0].Text)
	assert.Equal(t, by, answers[0].By)
	m := c.message(answer)
	assert.Equal(t, run.MessageConsumed, m.Status, "the answer is the run's message")
	require.NotNil(t, m.RunID)
	assert.Equal(t, c.parked.ID, *m.RunID)
}

func TestWhatsAppApproval_DMFirstMessageDecides(t *testing.T) {
	c := newRoom(t, "dm")
	yes := c.post(t, c.requester, "Ya!", "")
	later := c.post(t, c.requester, "tidak", "")
	c.assertDecided(t, c.drain(t), true, "", c.requester, yes)
	assert.Equal(t, run.MessagePending, c.message(later).Status, "later messages wait for the next turn")
}

func TestWhatsAppApproval_NonMatchRejectsAndStoresText(t *testing.T) {
	c := newRoom(t, "dm")
	no := c.post(t, c.requester, "tidak, 20000", "")
	c.assertDecided(t, c.drain(t), false, "tidak, 20000", c.requester, no)
	var got struct {
		Approved bool   `json:"approved"`
		Text     string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(c.f.rt.LastRequest().Input.Responses[0].Response, &got))
	assert.Equal(t, "tidak, 20000", got.Text, "the full text reaches the runtime")
}

func TestWhatsAppApproval_OtherMemberIgnored(t *testing.T) {
	c := newRoom(t, "group")
	quote := c.post(t, c.member, "ya", "wamid-prompt")
	plain := c.post(t, c.member, "ok", "")
	c.assertParked(t, quote, plain)

	yes := c.post(t, c.requester, "oke 👍", "")
	c.assertDecided(t, c.drain(t), true, "", c.requester, yes)
	assert.Equal(t, run.MessagePending, c.message(quote).Status, "the other member's messages stay pending")
}

func TestWhatsAppApproval_QuotedReplyDecides(t *testing.T) {
	c := newRoom(t, "group")
	quoted := c.post(t, c.requester, "no, it was 20000", "wamid-prompt")
	c.assertDecided(t, c.drain(t), false, "no, it was 20000", c.requester, quoted)
}

func TestWhatsAppApproval_UnquotedNonMatchWaits(t *testing.T) {
	c := newRoom(t, "group")
	other := c.post(t, c.requester, "berapa totalnya?", "")
	elsewhere := c.post(t, c.requester, "itu salah", "wamid-other")
	c.assertParked(t, other, elsewhere)

	yes := c.post(t, c.requester, "gas", "")
	c.assertDecided(t, c.drain(t), true, "", c.requester, yes)
	assert.Equal(t, run.MessagePending, c.message(other).Status)
}

func TestWhatsAppApproval_HeldMessagesRunInOrder(t *testing.T) {
	c := newRoom(t, "group")
	first := c.post(t, c.member, "halo semua", "")
	yes := c.post(t, c.requester, "sip", "")
	second := c.post(t, c.member, "jam berapa?", "")
	c.assertDecided(t, c.drain(t), true, "", c.requester, yes)

	next := c.drain(t)
	require.NotNil(t, next)
	assert.NotEqual(t, c.parked.ID, next.ID, "held messages run in a new turn")
	for _, id := range []uuid.UUID{first, second} {
		m := c.message(id)
		require.NotNil(t, m.RunID)
		assert.Equal(t, next.ID, *m.RunID)
	}
	parts := c.f.rt.LastRequest()
	assert.NotNil(t, parts.Input.Message)
}

func TestWhatsAppApproval_GroupQuoteOfAnotherMessageNeverDecides(t *testing.T) {
	c := newRoom(t, "group")
	reply := c.post(t, c.requester, "ya", "wamid-other")
	no := c.post(t, c.requester, "ok", "wamid-someone")
	c.assertParked(t, reply, no)

	yes := c.post(t, c.requester, "ya", "wamid-prompt")
	c.assertDecided(t, c.drain(t), true, "", c.requester, yes)
	assert.Equal(t, run.MessagePending, c.message(reply).Status, "a yes to someone else is not an answer")
}

func TestWhatsAppApproval_MessageBeforeThePromptNeverDecides(t *testing.T) {
	for _, kind := range []string{"dm", "group"} {
		t.Run(kind, func(t *testing.T) {
			c := newParked(t, kind)
			early := c.post(t, c.requester, "ok", "")
			c.sendPrompt()
			c.assertParked(t, early)

			yes := c.post(t, c.requester, "ya", "")
			c.assertDecided(t, c.drain(t), true, "", c.requester, yes)
			next := c.drain(t)
			require.NotNil(t, next, "the early message runs as a normal turn after the decision")
			m := c.message(early)
			require.NotNil(t, m.RunID)
			assert.Equal(t, next.ID, *m.RunID)
		})
	}
}

func TestWhatsAppApproval_NothingDecidesBeforeThePromptIsSent(t *testing.T) {
	c := newParked(t, "dm")
	yes := c.post(t, c.requester, "ya", "")
	c.assertParked(t, yes)
}

func TestWhatsAppApproval_ConsoleWinsTheRaceAndTheRunStillResumes(t *testing.T) {
	c := newRoom(t, "dm")
	answer := c.post(t, c.requester, "ya", "")
	c.a.BeforeChat = func() { c.a.Decide(c.parked.ID, "rejected", "from the console") }
	r := c.drain(t)
	require.NotNil(t, r, "the run resumes in the same drain, not at the next reconcile")
	assert.Equal(t, c.parked.ID, r.ID)
	assert.Empty(t, c.a.Answers(), "the chat lost the race")
	assert.Equal(t, run.MessagePending, c.message(answer).Status, "the losing answer stays a message")
}
