package boot

import (
	"context"

	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/run"
)

type runRetriever struct{ r knowledge.Retriever }

// Search implements run.KnowledgeRetriever over a knowledge backend, at the backend's default hit count.
func (a runRetriever) Search(ctx context.Context, q run.KnowledgeQuery) ([]run.KnowledgeHit, run.KnowledgeUsage, error) {
	hits, usage, err := a.r.Search(ctx, knowledge.Query{ProjectID: q.ProjectID, Sources: q.Sources, Text: q.Text})
	out := make([]run.KnowledgeHit, len(hits))
	for i, h := range hits {
		out[i] = run.KnowledgeHit{Text: h.Text, Title: h.Title, Heading: h.Heading, URL: h.URL, Page: h.Page, Score: h.Score, Source: h.Source}
	}
	return out, run.KnowledgeUsage{EmbeddingTokens: usage.EmbeddingTokens, RetrieveCalls: usage.RetrieveCalls}, err
}
