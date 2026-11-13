package knowledge

import (
	"context"

	"github.com/google/uuid"
)

// WithRetrievers sets the retriever of each enabled backend.
func (s *Service) WithRetrievers(r map[Backend]Retriever) *Service {
	s.retrievers = r
	return s
}

// TestSearch searches one source the way a turn does, for a person checking what an agent would find.
func (s *Service) TestSearch(ctx context.Context, src *Source, query string) ([]Hit, error) {
	r, ok := s.retrievers[src.Backend]
	if !ok {
		return nil, &InvalidSourceError{Reason: SourceBackendDisabled}
	}
	hits, _, err := r.Search(ctx, Query{ProjectID: src.ProjectID, Sources: []uuid.UUID{src.ID}, Text: query, K: RecommendedTopK})
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: test search", err)
	}
	return hits, nil
}
