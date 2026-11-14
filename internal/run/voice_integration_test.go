//go:build integration

package run_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/transcribe"
	"altalune.id/oagent/internal/testutil/fakes"
)

func (f *pgFixture) arriveVoice(t *testing.T, c chat, ext string) uuid.UUID {
	t.Helper()
	thread := f.arrive(t, c, ext, "[voice note]", time.Now())
	content, err := json.Marshal([]run.ContentPart{run.VoiceNotePart(ext, "audio/ogg")})
	require.NoError(t, err)
	f.exec(t, `UPDATE `+f.schema+`.messages SET body = body || jsonb_build_object('content', $2::jsonb, 'channel', '{"type":"audio","voice":true}'::jsonb)
		WHERE thread_id = $1 AND external_message_id = $3`, thread, string(content), ext)
	return thread
}

func TestStore_SetTakenMessageBodyGuards(t *testing.T) {
	p := newPgRunner(t)
	c := p.chat(t, p.s, "dm")
	thread := p.arriveVoice(t, c, "wamid-1")
	ctx := in(p.s.org)
	token := p.claimed(t, p.s, thread)
	r := run.New(p.s.org, p.s.project, thread, p.s.version, time.Now())
	taken, err := p.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.Len(t, taken, 1)
	msg := taken[0].ID
	body := json.RawMessage(`{"id":"wamid-1","role":"user","content":[{"type":"audio","id":"wamid-1","metadata":{"transcript":{"status":"done","text":"halo","seconds":3}}}]}`)

	assert.True(t, run.IsLeaseLostError(p.store.SetTakenMessageBody(ctx, "not-the-lease", r.ID, msg, body, 3)), "a wrong token is fenced out")
	assert.Error(t, p.store.SetTakenMessageBody(ctx, token, uuid.New(), msg, body, 3), "an unknown run")
	assert.Error(t, p.store.SetTakenMessageBody(in(uuid.New()), token, r.ID, msg, body, 3), "another org sees no run")
	_, err = p.store.AppendMessage(ctx, thread, userMsg("later", ""))
	require.NoError(t, err)
	later := p.count(t, `SELECT count(*) FROM `+p.schema+`.messages WHERE thread_id = $1 AND run_id IS NULL`, thread)
	require.Equal(t, 1, later)
	var pending uuid.UUID
	require.NoError(t, p.admin.QueryRowContext(t.Context(), `SELECT id FROM `+p.schema+`.messages WHERE thread_id = $1 AND run_id IS NULL`, thread).Scan(&pending))
	assert.Error(t, p.store.SetTakenMessageBody(ctx, token, r.ID, pending, body, 3), "a message the run did not take")
	assert.Equal(t, 0, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE id = $1 AND transcription_seconds > 0`, r.ID), "no refused write meters")

	require.NoError(t, p.store.SetTakenMessageBody(ctx, token, r.ID, msg, body, 3))
	require.NoError(t, p.store.SetTakenMessageBody(ctx, token, r.ID, msg, body, 2))
	got, err := p.store.RunByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), got.Usage.TranscriptionSeconds, "the seconds add up in the database, not in memory")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.messages WHERE id = $1 AND body->'content'->0->'metadata'->'transcript'->>'text' = 'halo'`, msg))
}

type driverTranscriber struct{}

func (driverTranscriber) Transcribe(ctx context.Context, m run.ResolvedModel, model string, audio []byte, mime string) (string, error) {
	c, err := transcribe.New(transcribe.Config{BaseURL: m.BaseURL, APIKey: m.APIKey, Model: model, AllowPrivateHosts: true})
	if err != nil {
		return "", err
	}
	return c.Transcribe(ctx, audio, mime)
}

func TestVoice_EndToEnd(t *testing.T) {
	stt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"text":"catat kopi dua gelas"}`))
	}))
	t.Cleanup(stt.Close)
	data, err := os.ReadFile("testdata/voice.ogg")
	require.NoError(t, err)

	for _, c := range []struct {
		name     string
		capCents int32
		status   string
		seconds  int64
	}{
		{"transcribed", 0, "done", 3},
		{"over the budget", 1, "skipped", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newPgRunner(t)
			provider, sp := uuid.New(), uuid.New()
			version := &run.AgentVersion{ID: p.s.version, AgentID: p.s.agent, Version: 1, Model: "gpt-4o", ProviderID: provider, MaxCostCents: c.capCents,
				TranscriptionModel: run.TranscriptionModel{ProviderID: sp, Model: "whisper-1"}}
			p.svc.WithAgents(&fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{version.ID: version}}).
				WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{
					provider: {"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test", Prices: &run.Prices{Input: new(int64(1)), Output: new(int64(1)), CacheRead: new(int64(0)), CacheWrite: new(int64(0))}}},
					sp:       {"whisper-1": {BaseURL: stt.URL + "/v1", APIKey: "sk-stt", Prices: &run.Prices{Transcription: new(int64(6_000_000))}}},
				}}).
				WithMedia(&media{data: data, mime: "audio/ogg"}).
				WithTranscriber(driverTranscriber{})
			ch := p.chat(t, p.s, "dm")
			thread := p.arriveVoice(t, ch, "wamid-1")

			r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
			require.NoError(t, err)
			assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.messages WHERE thread_id = $1 AND body->'content'->0->'metadata'->'transcript'->>'status' = $2`, thread, c.status))
			got, err := p.store.RunByID(in(p.s.org), r.ID)
			require.NoError(t, err)
			assert.Equal(t, c.seconds, got.Usage.TranscriptionSeconds)
			if c.status != "done" {
				assert.Empty(t, p.rt.Requests, "a skipped voice-only turn gets the set reply")
				return
			}
			assert.Equal(t, "[voice note] catat kopi dua gelas", inputContent(t, p.rt.LastRequest()))
			require.NotNil(t, got.Prices.Transcription)
			assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE id = $1 AND cost_micros >= 300000`, r.ID), "3 s at 6 USD per minute")
		})
	}
}
