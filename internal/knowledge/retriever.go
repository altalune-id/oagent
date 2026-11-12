package knowledge

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// The search bounds every Retriever keeps.
const (
	DefaultSearchK = 5
	MaxSearchK     = 20
	MinSearchScore = 0.2
)

// Query asks for the passages of Sources in ProjectID closest to Text; K above MaxSearchK or below 1 is clamped.
type Query struct {
	ProjectID uuid.UUID
	Sources   []uuid.UUID
	Text      string
	K         int
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

// Retriever searches one backend: hits best first, at most K, none below MinSearchScore, none outside q's project and sources.
type Retriever interface {
	Search(ctx context.Context, q Query) ([]Hit, Usage, error)
}

// SearchK clamps a requested K to the search bounds.
func SearchK(k int) int {
	if k < 1 {
		return DefaultSearchK
	}
	return min(k, MaxSearchK)
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
