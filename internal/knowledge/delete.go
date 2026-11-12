package knowledge

import (
	"context"

	"github.com/google/uuid"
)

// WithBindings makes DeleteSource refuse a source a published agent version searches, and ListSources name those agents; nil skips both.
func (s *Service) WithBindings(b Bindings) *Service {
	s.bindings = b
	return s
}

// DeleteSource removes a source no published agent version searches; its files go to the storage sweep in the same unit of work.
func (s *Service) DeleteSource(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.store.LockSource(ctx, id); err != nil {
			return err
		}
		if s.bindings != nil {
			inUse, err := s.bindings.SourceInUse(ctx, id)
			if err != nil {
				return err
			}
			if inUse {
				return &SourceInUseError{}
			}
		}
		files, err := s.store.SourceFileIDs(ctx, id)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := s.files.MarkDeleting(ctx, f); err != nil {
				return err
			}
		}
		return s.store.DeleteSource(ctx, id)
	})
	if err != nil {
		return s.errOrReport(ctx, "knowledge: delete source", err)
	}
	return nil
}

// DeleteDocument removes a document and its chunks; its file goes to the storage sweep in the same unit of work.
func (s *Service) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		doc, err := s.store.LockDocument(ctx, id)
		if err != nil {
			return err
		}
		if doc.FileID != nil {
			if err := s.files.MarkDeleting(ctx, *doc.FileID); err != nil {
				return err
			}
		}
		return s.store.DeleteDocument(ctx, id)
	})
	if err != nil {
		return s.errOrReport(ctx, "knowledge: delete document", err)
	}
	return nil
}

// SourceExists checks a source is in projectID, for agent binding; any other answers *SourceNotFoundError.
func (s *Service) SourceExists(ctx context.Context, projectID, sourceID uuid.UUID) error {
	src, err := s.store.SourceByID(ctx, sourceID)
	if err != nil {
		return s.errOrReport(ctx, "knowledge: source exists", err)
	}
	if src.ProjectID != projectID {
		return &SourceNotFoundError{}
	}
	return nil
}

// LockSourcesShared share-locks sources while an agent version that searches them is published.
func (s *Service) LockSourcesShared(ctx context.Context, ids []uuid.UUID) error {
	return s.errOrReport(ctx, "knowledge: lock sources", s.store.LockSourcesShared(ctx, ids))
}

func (s *Service) usedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]UsedBy, error) {
	if s.bindings == nil {
		return nil, nil
	}
	return s.bindings.SourcesUsedBy(ctx, projectID)
}
