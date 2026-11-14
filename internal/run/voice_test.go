package run_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type sttCall struct {
	model, mime string
	audio       []byte
	m           run.ResolvedModel
}

type transcriber struct {
	mu    sync.Mutex
	calls []sttCall
	text  string
	err   error
	hook  func(ctx context.Context, n int) error
}

func (s *transcriber) Transcribe(ctx context.Context, m run.ResolvedModel, model string, audio []byte, mime string) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, sttCall{model: model, mime: mime, audio: audio, m: m})
	n := len(s.calls)
	s.mu.Unlock()
	if s.hook != nil {
		if err := s.hook(ctx, n); err != nil {
			return "", err
		}
	}
	return s.text, s.err
}

func (s *transcriber) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func oggOf(t *testing.T, d time.Duration) []byte {
	t.Helper()
	page := func(granule int64, body []byte) []byte {
		h := make([]byte, 27)
		copy(h, "OggS")
		if granule == 0 {
			h[5] = 2
		}
		binary.LittleEndian.PutUint64(h[6:14], uint64(granule))
		h[26] = 1
		return append(append(h, byte(len(body))), body...)
	}
	head := make([]byte, 19)
	copy(head, "OpusHead")
	return append(page(0, head), page(int64(d/time.Second)*48_000+int64(d%time.Second)*48_000/int64(time.Second), []byte{1})...)
}

type voiceFixture struct {
	*svcFixture
	md  *media
	stt *transcriber
	sp  uuid.UUID
}

func newVoice(t *testing.T) *voiceFixture {
	t.Helper()
	f := newSvc(t)
	md, _ := f.channel(t, "dm", true)
	data, err := os.ReadFile("testdata/voice.ogg")
	require.NoError(t, err)
	md.data, md.mime = data, "audio/ogg"
	v := &voiceFixture{svcFixture: f, md: md, stt: &transcriber{text: "catat kopi dua gelas"}, sp: uuid.New()}
	f.providers.Models[v.sp] = map[string]run.ResolvedModel{
		"whisper-1": {BaseURL: "https://stt.example.com/v1", APIKey: "sk-stt", Prices: &run.Prices{Transcription: new(int64(6_000))}},
	}
	f.version.TranscriptionModel = run.TranscriptionModel{ProviderID: v.sp, Model: "whisper-1"}
	f.svc.WithTranscriber(v.stt)
	return v
}

func (v *voiceFixture) stored(t *testing.T, ext string) map[string]any {
	t.Helper()
	for _, m := range v.store.Messages {
		if m.ExternalMessageID != ext {
			continue
		}
		var body struct {
			Content []struct {
				Type     string `json:"type"`
				Metadata *struct {
					Transcript map[string]any `json:"transcript"`
				} `json:"metadata"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(m.Body, &body))
		for _, p := range body.Content {
			if p.Type == "audio" && p.Metadata != nil {
				return p.Metadata.Transcript
			}
		}
		return nil
	}
	t.Fatalf("no message %s", ext)
	return nil
}

func TestVoice_TranscribesAndAnswers(t *testing.T) {
	v := newVoice(t)
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	r := v.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})

	require.Equal(t, 1, v.stt.count())
	call := v.stt.calls[0]
	assert.Equal(t, "whisper-1", call.model)
	assert.Equal(t, "audio/ogg", call.mime)
	assert.Equal(t, v.md.data, call.audio)
	assert.Equal(t, "https://stt.example.com/v1", call.m.BaseURL)
	assert.Equal(t, int64(5<<20), v.md.fetched[0].maxBytes)
	assert.Equal(t, map[string]any{"status": "done", "text": "catat kopi dua gelas", "seconds": float64(3)}, v.stored(t, "wamid-1"))
	assert.Equal(t, "[voice note] catat kopi dua gelas", inputContent(t, v.rt.LastRequest()))
	assert.Equal(t, int64(3), r.Usage.TranscriptionSeconds, "2.5 s bills 3 s")
	assert.Equal(t, int64(3), v.store.Runs[r.ID].Usage.TranscriptionSeconds)
	assert.Equal(t, int64(6_000), *r.Prices.Transcription, "the transcription price is pinned at open")
}

func TestVoice_ShortClipBillsOneSecond(t *testing.T) {
	v := newVoice(t)
	v.md.data = oggOf(t, 400*time.Millisecond)
	v.version.MaxCostCents = 100
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	r := v.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, float64(1), v.stored(t, "wamid-1")["seconds"])
	assert.Equal(t, int64(1), r.Usage.TranscriptionSeconds)
	micros, priced := run.Cost(run.Usage{TranscriptionSeconds: r.Usage.TranscriptionSeconds}, r.Prices)
	require.True(t, priced)
	assert.Equal(t, int64(100), micros)
}

func TestVoice_Outcomes(t *testing.T) {
	for name, c := range map[string]struct {
		edit   func(t *testing.T, v *voiceFixture)
		status string
		reason string
		called bool
	}{
		"too large": {func(_ *testing.T, v *voiceFixture) {
			v.md.err = fmt.Errorf("%w: %w", run.ErrMediaTooLarge, errors.New("channel: media over the size limit"))
		}, "skipped", "too_large", false},
		"refused type": {func(_ *testing.T, v *voiceFixture) {
			v.md.err = fmt.Errorf("%w: %w", run.ErrMediaType, errors.New("channel: unsupported media type"))
		}, "skipped", "unsupported", false},
		"not ogg":         {func(_ *testing.T, v *voiceFixture) { v.md.mime = "audio/mpeg" }, "skipped", "unsupported", false},
		"fetch failed":    {func(_ *testing.T, v *voiceFixture) { v.md.err = errors.New("openwa down") }, "failed", "failed", false},
		"unreadable":      {func(_ *testing.T, v *voiceFixture) { v.md.data = []byte("ID3 not ogg") }, "skipped", "unreadable", false},
		"too long":        {func(t *testing.T, v *voiceFixture) { v.md.data = oggOf(t, 5*time.Minute+time.Second) }, "skipped", "too_long", false},
		"rate limited":    {func(_ *testing.T, v *voiceFixture) { v.stt.err = fmt.Errorf("%w: 429", run.ErrTranscribeRateLimited) }, "failed", "rate_limited", true},
		"call failed":     {func(_ *testing.T, v *voiceFixture) { v.stt.err = errors.New("stt down") }, "failed", "failed", true},
		"empty text":      {func(_ *testing.T, v *voiceFixture) { v.stt.text = "  " }, "failed", "unreadable", true},
		"unresolved":      {func(_ *testing.T, v *voiceFixture) { delete(v.providers.Models, v.sp) }, "failed", "failed", false},
		"over the budget": {overBudget, "skipped", "over_budget", false},
		"unpriced":        {unpriced, "skipped", "unpriced", false},
		"capped on an unpriced chat model": {func(_ *testing.T, v *voiceFixture) {
			v.version.MaxCostCents = 100
			chat := v.providers.Models[v.provider]["gpt-4o"]
			chat.Prices = nil
			v.providers.Models[v.provider]["gpt-4o"] = chat
		}, "skipped", "unpriced", false},
	} {
		t.Run(name, func(t *testing.T) {
			v := newVoice(t)
			c.edit(t, v)
			v.post(t, voicePartMsg("wamid-1", nil, "", nil))
			r := v.execute(t)
			tr := v.stored(t, "wamid-1")
			assert.Equal(t, c.status, tr["status"])
			assert.Equal(t, c.reason, tr["reason"])
			assert.NotContains(t, tr, "text")
			assert.Equal(t, c.called, v.stt.count() > 0)
			assert.Zero(t, r.Usage.TranscriptionSeconds, "only a transcript is metered")
			assert.Empty(t, v.rt.Requests, "an untranscribed voice-only DM gets the set reply, not the model")
			assert.Equal(t, run.StatusCompleted, r.Status)
		})
	}
}

func overBudget(_ *testing.T, v *voiceFixture) {
	v.version.MaxCostCents = 1
	v.providers.Models[v.sp]["whisper-1"] = run.ResolvedModel{Prices: &run.Prices{Transcription: new(int64(1_000_000))}}
}

func unpriced(_ *testing.T, v *voiceFixture) {
	chat := v.providers.Models[v.provider]["gpt-4o"]
	prices := *chat.Prices
	prices.Transcription = new(int64(1))
	chat.Prices = &prices
	v.providers.Models[v.provider]["gpt-4o"] = chat
	v.providers.Models[v.sp]["whisper-1"] = run.ResolvedModel{BaseURL: "https://stt.example.com/v1"}
}

func TestVoice_NoCallWithoutAConnectionOrAModel(t *testing.T) {
	v := newVoice(t)
	v.store.Connections = nil
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	v.execute(t)
	assert.Zero(t, v.stt.count(), "no connection")
	assert.Nil(t, v.stored(t, "wamid-1"), "the part stays untouched")

	w := newVoice(t)
	w.version.TranscriptionModel = run.TranscriptionModel{}
	w.post(t, voicePartMsg("wamid-1", nil, "", nil))
	r := w.execute(t)
	assert.Zero(t, w.stt.count(), "voice notes off")
	assert.Nil(t, w.stored(t, "wamid-1"))
	assert.Nil(t, r.Prices.Transcription, "no transcription price is pinned when voice notes are off")
}

func TestVoice_StoredTranscriptIsNeverRedone(t *testing.T) {
	v := newVoice(t)
	v.post(t, voicePartMsg("wamid-1", map[string]any{"status": "done", "text": "sudah", "seconds": 2}, "", nil))
	v.post(t, voicePartMsg("wamid-2", map[string]any{"status": "failed", "reason": "failed"}, "", nil))
	v.post(t, voicePartMsg("wamid-3", nil, "", nil))
	v.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	assert.Equal(t, 1, v.stt.count(), "only the untouched part is transcribed")
	assert.Equal(t, "[voice note] sudah\n\n[voice note]\n\n[voice note] catat kopi dua gelas", inputContent(t, v.rt.LastRequest()))
}

func TestVoice_LeaseLostStopsTheTurn(t *testing.T) {
	v := newVoice(t)
	v.stt.hook = func(context.Context, int) error {
		v.store.Threads[v.thread].LeaseOwner = "someone-else"
		return nil
	}
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	token := v.claim(t)
	r, err := v.svc.Execute(v.ctx, v.thread, token)
	assert.True(t, run.IsLeaseLostError(err), "got %v", err)
	assert.Nil(t, r)
	assert.Empty(t, v.rt.Requests, "no runtime call after the lease is gone")
	assert.Nil(t, v.stored(t, "wamid-1"))
}

func TestVoice_ParentCancelWritesNothing(t *testing.T) {
	v := newVoice(t)
	ctx, cancel := context.WithCancel(v.ctx)
	defer cancel()
	v.stt.hook = func(context.Context, int) error {
		cancel()
		return context.Canceled
	}
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	token := v.claim(t)
	r, err := v.svc.Execute(ctx, v.thread, token)
	require.NoError(t, err)
	assert.Equal(t, run.StatusInterrupted, r.Status, "a cancelled turn is retried, not answered")
	assert.NotEqual(t, run.ReplyDue, r.ReplyState, "no voice set reply for a note the retry will transcribe")
	assert.Empty(t, v.assistantTexts(t))
	assert.Nil(t, v.stored(t, "wamid-1"), "a cancelled turn stores no outcome, so the next run transcribes it")
}

type cancelOnResolve struct {
	run.Providers
	stt    uuid.UUID
	cancel context.CancelFunc
	calls  int
}

func (c *cancelOnResolve) ResolveModel(ctx context.Context, providerID uuid.UUID, model string) (run.ResolvedModel, error) {
	if providerID == c.stt {
		if c.calls++; c.calls == 2 {
			c.cancel()
			return run.ResolvedModel{}, context.Canceled
		}
	}
	return c.Providers.ResolveModel(ctx, providerID, model)
}

func TestVoice_ParentCancelDuringResolveIsRetried(t *testing.T) {
	v := newVoice(t)
	ctx, cancel := context.WithCancel(v.ctx)
	defer cancel()
	v.svc.WithProviders(&cancelOnResolve{Providers: v.providers, stt: v.sp, cancel: cancel})
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	token := v.claim(t)
	r, err := v.svc.Execute(ctx, v.thread, token)
	require.NoError(t, err)
	assert.Equal(t, run.StatusInterrupted, r.Status, "a resolve cut short by the parent is retried, not stored as failed")
	assert.Zero(t, v.stt.count())
	assert.Nil(t, v.stored(t, "wamid-1"))
	assert.Empty(t, v.assistantTexts(t))
}

func TestVoice_StepDeadlineFailsTheRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := newVoice(t)
		v.stt.hook = func(ctx context.Context, _ int) error {
			<-ctx.Done()
			return ctx.Err()
		}
		for _, ext := range []string{"wamid-1", "wamid-2", "wamid-3"} {
			v.post(t, voicePartMsg(ext, nil, "", nil))
		}
		r := v.execute(t)
		assert.Equal(t, 1, v.stt.count(), "the first call used up the 30 s step, so the rest get no call")
		for _, ext := range []string{"wamid-1", "wamid-2", "wamid-3"} {
			assert.Equal(t, map[string]any{"status": "failed", "reason": "failed"}, v.stored(t, ext), ext)
		}
		assert.Equal(t, run.StatusCompleted, r.Status)
	})
}

func TestVoice_ParentDeadlineDuringTheStepWritesNothingMore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := newVoice(t)
		v.stt.hook = func(ctx context.Context, n int) error {
			if n == 1 {
				return errors.New("stt down")
			}
			<-ctx.Done()
			return ctx.Err()
		}
		v.post(t, voicePartMsg("wamid-1", nil, "", nil))
		v.post(t, voicePartMsg("wamid-2", nil, "", nil))
		ctx, cancel := context.WithTimeout(v.ctx, 25*time.Second)
		defer cancel()
		token := v.claim(t)
		r, err := v.svc.Execute(ctx, v.thread, token)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"status": "failed", "reason": "failed"}, v.stored(t, "wamid-1"))
		assert.Nil(t, v.stored(t, "wamid-2"), "the parent ended during the second call, so nothing is stored for it")
		assert.Equal(t, run.StatusInterrupted, r.Status)
	})
}

func TestVoice_TranscriptNeverLogged(t *testing.T) {
	v := newVoice(t)
	var logs bytes.Buffer
	v.svc.WithLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	v.post(t, voicePartMsg("wamid-1", nil, "", nil))
	v.execute(t, fakes.RuntimeReply{Stream: stream(t, "plain_reply")})
	v.stt.err = errors.New("stt down: catat kopi")
	v.post(t, voicePartMsg("wamid-2", nil, "", nil))
	v.execute(t)
	assert.NotContains(t, logs.String(), "catat kopi")
	assert.NotContains(t, logs.String(), "wamid-")
	assert.Contains(t, logs.String(), `"reason":"failed"`)
}
