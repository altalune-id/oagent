package knowledge

import (
	"context"

	"github.com/google/uuid"
)

// DocumentStats is a document's file size, 0 for a crawled page, and its passage count.
type DocumentStats struct {
	SizeBytes int64
	Chunks    int
}

// ViewReader reads the counts shown next to one source or document.
type ViewReader interface {
	SourceDocumentCounts(ctx context.Context, sourceID uuid.UUID) (DocumentCounts, error)
	DocumentStats(ctx context.Context, documentIDs []uuid.UUID) (map[uuid.UUID]DocumentStats, error)
}

// SourceView returns src with its document counts and the agents whose published version searches it.
func (s *Service) SourceView(ctx context.Context, src *Source) (SourceView, error) {
	counts, err := s.store.SourceDocumentCounts(ctx, src.ID)
	if err != nil {
		return SourceView{}, s.errOrReport(ctx, "knowledge: source document counts", err)
	}
	used, err := s.usedBy(ctx, src.ProjectID)
	if err != nil {
		return SourceView{}, s.errOrReport(ctx, "knowledge: used by", err)
	}
	return SourceView{Source: src, Documents: counts, UsedBy: used[src.ID]}, nil
}

// DocumentStats returns the stats of each document in ids; a document with no row is absent from the map.
func (s *Service) DocumentStats(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]DocumentStats, error) {
	out, err := s.store.DocumentStats(ctx, ids)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: document stats", err)
	}
	return out, nil
}
