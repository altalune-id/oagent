package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type pendingVoice struct {
	msg, part int
	p         ContentPart
}

func pendingVoices(taken []Message) []pendingVoice {
	var out []pendingVoice
	for i, m := range taken {
		if m.Role != RoleUser {
			continue
		}
		var u agUIMessage
		if json.Unmarshal(m.Body, &u) != nil {
			continue
		}
		parts, err := userParts(u.Content)
		if err != nil {
			continue
		}
		for j, p := range parts {
			if p.Type == "audio" && p.transcriptOf() == nil {
				out = append(out, pendingVoice{msg: i, part: j, p: p})
			}
		}
	}
	return out
}

// SECURITY: fail-open; logs carry the run public id and the reason only, never the transcript, the audio or the phone.
func (s *Service) transcribe(ctx context.Context, t *turn) error {
	tm := t.version.TranscriptionModel
	if s.transcriber == nil || s.media == nil || t.thread.ConnectionID == uuid.Nil || tm.ProviderID == uuid.Nil {
		return nil
	}
	pending := pendingVoices(t.taken)
	if len(pending) == 0 {
		return nil
	}
	var fixed *transcript
	m, err := s.providers.ResolveModel(ctx, tm.ProviderID, tm.Model)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		fixed = &transcript{Status: transcriptFailed, Reason: "failed"}
	case t.run.Prices != nil && t.run.Prices.Transcription == nil, t.run.Prices == nil && t.version.MaxCostCents > 0:
		// SECURITY: a priced run never takes on cost it cannot price, so the budget cap holds.
		fixed = &transcript{Status: transcriptSkipped, Reason: "unpriced"}
	}
	step, cancel := context.WithTimeout(ctx, voiceStepTime)
	defer cancel()
	for _, v := range pending {
		tr := transcript{Status: transcriptFailed, Reason: "failed"}
		switch {
		case fixed != nil:
			tr = *fixed
		case step.Err() == nil:
			tr = s.hear(step, t, m, tm.Model, v.p)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.storeTranscript(ctx, t, v, tr); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) hear(ctx context.Context, t *turn, m ResolvedModel, model string, p ContentPart) transcript {
	skipped := func(reason string) transcript { return transcript{Status: transcriptSkipped, Reason: reason} }
	failed := func(reason string) transcript { return transcript{Status: transcriptFailed, Reason: reason} }
	data, mime, err := s.audioBytes(ctx, t, p)
	switch {
	case errors.Is(err, ErrMediaTooLarge):
		return skipped("too_large")
	case errors.Is(err, ErrMediaType):
		return skipped("unsupported")
	case err != nil:
		return failed("failed")
	case mime != voiceMime:
		return skipped("unsupported")
	}
	length, err := oggDuration(data)
	switch {
	case err != nil:
		return skipped("unreadable")
	case length > MaxVoiceLength:
		return skipped("too_long")
	}
	billed := max(int64((length+time.Second-1)/time.Second), 1)
	if b := s.budget(t); b != nil && classCost(billed, *t.run.Prices.Transcription, secondsPerPrice) > b.RemainingMicros {
		return skipped("over_budget")
	}
	text, err := s.transcriber.Transcribe(ctx, m, model, data, mime)
	switch {
	case errors.Is(err, ErrTranscribeRateLimited):
		return failed("rate_limited")
	case err != nil:
		return failed("failed")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return failed("unreadable")
	}
	if utf8.RuneCountInString(text) > maxMessageRunes {
		text = string([]rune(text)[:maxMessageRunes])
	}
	return transcript{Status: transcriptDone, Text: text, Seconds: billed}
}

func (s *Service) audioBytes(ctx context.Context, t *turn, p ContentPart) (data []byte, mime string, err error) {
	stored := ""
	if p.Source != nil {
		stored = p.Source.MimeType
	}
	return s.media.Fetch(ctx, t.thread.ConnectionID, p.ID, stored, MaxVoiceBytes)
}

func (s *Service) storeTranscript(ctx context.Context, t *turn, v pendingVoice, tr transcript) error {
	body, err := withTranscript(t.taken[v.msg].Body, v.part, tr)
	if err != nil {
		return err
	}
	if err := s.store.SetTakenMessageBody(ctx, t.token, t.run.ID, t.taken[v.msg].ID, body, tr.Seconds); err != nil {
		return err
	}
	t.taken[v.msg].Body = body
	t.run.Usage.TranscriptionSeconds += tr.Seconds
	if s.transcribed != nil {
		s.transcribed.Add(ctx, 1, metric.WithAttributes(attribute.String("status", tr.Status), attribute.String("reason", tr.Reason)))
	}
	if tr.Status != transcriptDone {
		s.log.InfoContext(ctx, "run: voice note not transcribed; the agent sees [voice note]", "run", t.run.PublicID, "reason", tr.Reason)
	}
	return nil
}

func (s *Service) pinTranscription(ctx context.Context, t *turn, p *Prices) *Prices {
	if p == nil {
		return nil
	}
	pinned := *p
	pinned.Transcription = nil
	if tm := t.version.TranscriptionModel; tm.ProviderID != uuid.Nil {
		if m, err := s.providers.ResolveModel(ctx, tm.ProviderID, tm.Model); err == nil && m.Prices != nil {
			pinned.Transcription = m.Prices.Transcription
		}
	}
	return &pinned
}
