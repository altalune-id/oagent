package boot

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/run"
)

type recordingRetriever struct {
	got  knowledge.Query
	hits []knowledge.Hit
	use  knowledge.Usage
	err  error
}

func (r *recordingRetriever) Search(_ context.Context, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage, error) {
	r.got = q
	return r.hits, r.use, r.err
}

func TestRunRetriever_MapsTheQueryHitsAndUsage(t *testing.T) {
	project, src := uuid.New(), uuid.New()
	inner := &recordingRetriever{
		hits: []knowledge.Hit{{Text: "We refund in 7 days.", Title: "FAQ", Heading: "Refunds", Page: 3, URL: "https://e.com/faq", Score: 0.8, Source: src}},
		use:  knowledge.Usage{EmbeddingTokens: 12, RetrieveCalls: 1},
		err:  errors.New("partial"),
	}
	floor := 0.4
	hits, usage, err := runRetriever{inner}.Search(t.Context(), run.KnowledgeQuery{ProjectID: project, Sources: []uuid.UUID{src}, Text: "refunds", Context: "cards?\nrefunds", K: 3, MinScore: &floor})

	assert.Equal(t, knowledge.Query{ProjectID: project, Sources: []uuid.UUID{src}, Text: "refunds", Context: "cards?\nrefunds", K: 3, MinScore: &floor}, inner.got)
	assert.Equal(t, []run.KnowledgeHit{{Text: "We refund in 7 days.", Title: "FAQ", Heading: "Refunds", Page: 3, URL: "https://e.com/faq", Score: 0.8, Source: src}}, hits)
	assert.Equal(t, run.KnowledgeUsage{EmbeddingTokens: 12, RetrieveCalls: 1}, usage)
	assert.EqualError(t, err, "partial", "an error passes through with what was spent")
}

func TestRunKnowledgeTuning_ResolvesTheBudgetAndPassesTheRest(t *testing.T) {
	assert.Equal(t, run.KnowledgeTuning{MaxBytes: knowledge.RecommendedMaxTokens * 4}, runKnowledgeTuning(agent.KnowledgeTuning{}),
		"unset: the recommended budget, and K and MinScore left to the retriever")
	zero := 0.0
	got := runKnowledgeTuning(agent.KnowledgeTuning{TopK: 3, MinScore: &zero, MaxTokens: 1000})
	assert.Equal(t, 3, got.K)
	require.NotNil(t, got.MinScore, "a set minimum of 0 is kept")
	assert.Zero(t, *got.MinScore)
	assert.Equal(t, 4000, got.MaxBytes, "tokens × 4 bytes")
	assert.Equal(t, knowledge.MaxMaxTokens*4, runKnowledgeTuning(agent.KnowledgeTuning{MaxTokens: 99999}).MaxBytes, "the budget is clamped like the effective view")
}
