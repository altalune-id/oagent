package knowledge

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/knowledge/embed"
)

// EmbedderFor opens the embedder of a project's settings: its provider's model, at the stored vector size.
func (s *Service) EmbedderFor(ctx context.Context, st *Settings) (Embedder, error) {
	m, err := s.embeddingModel(ctx, st.ProviderID, st.Model)
	if err != nil {
		return nil, err
	}
	return s.embedder(m, st.Model, st.Dims)
}

// EmbeddingPrice is the input price of a project's embedding model, nil when the project has none.
func (s *Service) EmbeddingPrice(ctx context.Context, projectID uuid.UUID) (*int64, error) {
	st, err := s.store.Settings(ctx, projectID)
	if IsEmbeddingModelNotSetError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := s.models.EmbeddingModel(ctx, st.ProviderID, st.Model)
	if err != nil {
		return nil, err
	}
	return m.InputPriceMicros, nil
}

// EmbedError maps an embeddings client error to an EmbeddingFailedError with its reason; any other error passes through.
func EmbedError(err error) error {
	e, ok := embed.AsError(err)
	if !ok {
		return err
	}
	reason := EmbeddingUnreachable
	switch e.Reason {
	case embed.Rejected:
		reason = EmbeddingRejected
	case embed.WrongDims:
		reason = EmbeddingWrongDims
	}
	return &EmbeddingFailedError{Reason: reason, Cause: err}
}
