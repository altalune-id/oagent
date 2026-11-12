package run

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/runtime"
)

const (
	knowledgeBudget        = 10 * time.Second
	maxKnowledgeQueryRunes = 2000
)

// MaxRetrievesPerRun caps the paid knowledge searches of one run over all its legs.
const MaxRetrievesPerRun = 8

// NOTE: the embedding price is pinned with the run's other prices, so a later price change never reprices a run.
func (s *Service) pinPrices(ctx context.Context, t *turn, p *Prices) *Prices {
	if p == nil || s.embedPrices == nil || s.bindings == nil {
		return p
	}
	if sources, err := s.bindings.BoundSources(ctx, t.version.ID); err != nil || len(sources) == 0 {
		return p
	}
	pinned := *p
	price, err := s.embedPrices.EmbeddingPrice(ctx, t.run.ProjectID)
	if err != nil {
		s.log.WarnContext(ctx, "run: embedding price; the run searches no knowledge", "run", t.run.PublicID, "err", err.Error())
		return &pinned
	}
	pinned.Embedding = price
	return &pinned
}

// SECURITY: fail-open; a failed search logs the sources by public id, never the query or a passage, and the turn runs without knowledge.
func (s *Service) retrieve(ctx context.Context, t *turn) {
	t.knowledge = nil
	// SECURITY: a priced run never takes on tokens it cannot price, so the budget cap holds.
	if s.bindings == nil || s.retriever == nil || (t.run.Prices != nil && t.run.Prices.Embedding == nil) ||
		t.run.KnowledgeRetrieveCalls >= MaxRetrievesPerRun {
		return
	}
	sources, err := s.bindings.BoundSources(ctx, t.version.ID)
	if err != nil {
		s.retrieveFailed(ctx, t, nil, err)
		return
	}
	text := turnText(t.taken)
	if len(sources) == 0 || text == "" {
		return
	}
	ids := make([]uuid.UUID, len(sources))
	for i, src := range sources {
		ids[i] = src.ID
	}
	q := KnowledgeQuery{ProjectID: t.run.ProjectID, Sources: ids, Text: s.queryText(ctx, t, text)}
	searchCtx, cancel := context.WithTimeout(ctx, knowledgeBudget)
	defer cancel()
	start := time.Now()
	hits, usage, err := s.retriever.Search(searchCtx, q)
	ms := time.Since(start).Milliseconds()
	t.run.Usage.EmbeddingTokens += usage.EmbeddingTokens
	t.run.KnowledgeRetrieveCalls += int32(min(max(usage.RetrieveCalls, 0), math.MaxInt32))
	t.run.KnowledgeRetrieveMs += ms
	if s.retrieveMs != nil {
		s.retrieveMs.Record(ctx, ms)
	}
	if err != nil {
		s.retrieveFailed(ctx, t, sources, err)
		return
	}
	t.knowledge = passages(hits)
}

func (s *Service) retrieveFailed(ctx context.Context, t *turn, sources []KnowledgeSource, err error) {
	if ctx.Err() != nil {
		return
	}
	if s.retrieveErr != nil {
		s.retrieveErr.Add(ctx, 1)
	}
	ids := make([]string, len(sources))
	for i, src := range sources {
		ids[i] = src.PublicID
	}
	s.log.WarnContext(ctx, "run: knowledge search failed; the turn runs without it", "run", t.run.PublicID, "sources", ids, "err", err.Error())
}

func (s *Service) queryText(ctx context.Context, t *turn, text string) string {
	msgs, err := s.store.History(ctx, t.run.ThreadID, t.run.ID)
	if err != nil {
		s.log.WarnContext(ctx, "run: knowledge query without the previous message", "run", t.run.PublicID, "err", err.Error())
		msgs = nil
	}
	for _, m := range slices.Backward(msgs) {
		if m.Role != RoleUser {
			continue
		}
		if prev := userText(m); prev != "" {
			text = prev + "\n" + text
		}
		break
	}
	if n := utf8.RuneCountInString(text); n > maxKnowledgeQueryRunes {
		text = string([]rune(text)[n-maxKnowledgeQueryRunes:])
	}
	return text
}

func turnText(taken []Message) string {
	var lines []string
	for _, m := range taken {
		if m.Role != RoleUser {
			continue
		}
		if text := userText(m); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func userText(m Message) string {
	var u agUIMessage
	if json.Unmarshal(m.Body, &u) != nil {
		return ""
	}
	parts, err := userParts(u.Content)
	if err != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
			texts = append(texts, strings.TrimSpace(p.Text))
		}
	}
	return strings.Join(texts, "\n")
}

func passages(hits []KnowledgeHit) []runtime.Passage {
	out := make([]runtime.Passage, 0, len(hits))
	for _, h := range hits {
		p := runtime.Passage{Text: h.Text, Title: h.Title, Heading: h.Heading, URL: h.URL}
		if h.Page > 0 {
			p.Page = new(int32(min(h.Page, 1<<31-1)))
		}
		out = append(out, p)
	}
	return runtime.NormalizePassages(out)
}
