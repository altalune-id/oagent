package knowledge

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Query asks for the passages of Sources in ProjectID closest to Text; K is clamped to 1..MaxTopK; nil MinScore means RecommendedMinScore.
type Query struct {
	ProjectID uuid.UUID
	Sources   []uuid.UUID
	Text      string
	Context   string
	K         int
	MinScore  *float64
}

// Inputs are the texts a search embeds together: Text alone, then Context when it is set and differs.
func (q Query) Inputs() []string {
	if c := strings.TrimSpace(q.Context); c != "" && c != strings.TrimSpace(q.Text) {
		return []string{q.Text, q.Context}
	}
	return []string{q.Text}
}

// Hit is one retrieved passage with its document and source; Score is the cosine similarity. SECURITY: project content; never log it.
type Hit struct {
	Text    string
	Title   string
	Heading string
	Page    int
	URL     string
	Score   float64
	Source  uuid.UUID
}

// Usage is what one search spent.
type Usage struct {
	EmbeddingTokens int64
	RetrieveCalls   int
}

// Retriever searches one backend: hits best first, at most K, none below MinScoreOf(q), none outside q's project and sources.
type Retriever interface {
	Search(ctx context.Context, q Query) ([]Hit, Usage, error)
}

// MinScoreOf is q's minimum score clamped to 0..1, or RecommendedMinScore when q sets none.
func MinScoreOf(q Query) float64 {
	if q.MinScore == nil {
		return RecommendedMinScore
	}
	return min(max(*q.MinScore, 0), 1)
}

// SearchK clamps a requested K to the search bounds.
func SearchK(k int) int {
	if k < 1 {
		return RecommendedTopK
	}
	return min(k, MaxTopK)
}

// VectorText is v in pgvector's text form, for a $n::public.vector parameter.
func VectorText(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
