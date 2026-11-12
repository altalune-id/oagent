package boot

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

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
	hits, usage, err := runRetriever{inner}.Search(t.Context(), run.KnowledgeQuery{ProjectID: project, Sources: []uuid.UUID{src}, Text: "refunds"})

	assert.Equal(t, knowledge.Query{ProjectID: project, Sources: []uuid.UUID{src}, Text: "refunds"}, inner.got, "K is left to the backend's default")
	assert.Equal(t, []run.KnowledgeHit{{Text: "We refund in 7 days.", Title: "FAQ", Heading: "Refunds", Page: 3, URL: "https://e.com/faq", Score: 0.8, Source: src}}, hits)
	assert.Equal(t, run.KnowledgeUsage{EmbeddingTokens: 12, RetrieveCalls: 1}, usage)
	assert.EqualError(t, err, "partial", "an error passes through with what was spent")
}
