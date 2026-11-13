package boot

import (
	"context"

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/run"
)

type runRetriever struct{ r knowledge.Retriever }

// Search implements run.KnowledgeRetriever over a knowledge backend.
func (a runRetriever) Search(ctx context.Context, q run.KnowledgeQuery) ([]run.KnowledgeHit, run.KnowledgeUsage, error) {
	kq := knowledge.Query{ProjectID: q.ProjectID, Sources: q.Sources, Text: q.Text, Context: q.Context, K: q.K, MinScore: q.MinScore}
	hits, usage, err := a.r.Search(ctx, kq)
	out := make([]run.KnowledgeHit, len(hits))
	for i, h := range hits {
		out[i] = run.KnowledgeHit{Text: h.Text, Title: h.Title, Heading: h.Heading, URL: h.URL, Page: h.Page, Score: h.Score, Source: h.Source}
	}
	return out, run.KnowledgeUsage{EmbeddingTokens: usage.EmbeddingTokens, RetrieveCalls: usage.RetrieveCalls}, err
}

// SECURITY: the budget is resolved here, from internal/knowledge, so a version with no tuning still gets the recommended cap.
func runKnowledgeTuning(c agent.KnowledgeTuning) run.KnowledgeTuning {
	return run.KnowledgeTuning{K: int(c.TopK), MinScore: c.MinScore, MaxBytes: knowledge.MaxTokensOf(int(c.MaxTokens)) * 4}
}
