package knowledge

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge/embed"
)

// WithModels lets SetEmbeddingModel and the ingest worker open the project's embedding model.
func (s *Service) WithModels(m Models) *Service {
	s.models = m
	return s
}

// WithEmbedders builds embedders with fn instead of the guarded embeddings client; tests only.
func (s *Service) WithEmbedders(fn func(embed.Config) (Embedder, error)) *Service {
	s.newEmbedder = fn
	return s
}

// WithAllowPrivateHosts lets embeddings calls reach private addresses; self-hosted only.
func (s *Service) WithAllowPrivateHosts(allow bool) *Service {
	s.allowPrivateHosts = allow
	return s
}

// WithCrawlPrivateHosts lets the crawler reach private addresses. SECURITY: development only; tenants pick the seeds.
func (s *Service) WithCrawlPrivateHosts(allow bool) *Service {
	s.crawlPrivateHosts = allow
	return s
}

func newGuardedEmbedder(cfg embed.Config) (Embedder, error) { return embed.New(cfg) }

// Settings loads a project's embedding model, or *EmbeddingModelNotSetError.
func (s *Service) Settings(ctx context.Context, projectID uuid.UUID) (*Settings, error) {
	st, err := s.store.Settings(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: settings", err)
	}
	return st, nil
}

// EmbeddingModelOf names the model a project embeds with when it is providerID's, else "".
func (s *Service) EmbeddingModelOf(ctx context.Context, providerID uuid.UUID) (string, error) {
	m, err := s.store.EmbeddingModelOf(ctx, providerID)
	if err != nil {
		return "", s.errOrReport(ctx, "knowledge: embedding model of", err)
	}
	return m, nil
}

// SetEmbeddingModel checks and probes a provider's model, stores its size and, when the model changes, queues every document to re-embed.
func (s *Service) SetEmbeddingModel(ctx context.Context, projectID, providerID uuid.UUID, model string) (*Settings, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.embeddingModel(ctx, providerID, model)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: embedding model", err)
	}
	emb, err := s.embedder(m, model, 0)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: embedder", err)
	}
	dims, err := emb.Probe(ctx)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: probe", EmbedError(err))
	}
	if !slices.Contains(SupportedDims, dims) {
		return nil, &EmbeddingModelInvalidError{Reason: EmbeddingSize, Dims: dims}
	}
	next := &Settings{OrgID: tc.OrgID, ProjectID: projectID, ProviderID: providerID, Model: model, Dims: dims, UpdatedAt: s.now().UTC()}
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.models.LockProvider(ctx, providerID); err != nil {
			return err
		}
		if _, err := s.embeddingModel(ctx, providerID, model); err != nil {
			return err
		}
		prev, err := s.store.LockSettings(ctx, projectID)
		if err != nil && !IsEmbeddingModelNotSetError(err) {
			return err
		}
		if err := s.store.SaveSettings(ctx, next); err != nil {
			return err
		}
		if prev != nil && prev.ModelKey() == next.ModelKey() {
			return nil
		}
		return s.store.RequeueDocuments(ctx, projectID)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: set embedding model", err)
	}
	return next, nil
}

func (s *Service) embeddingModel(ctx context.Context, providerID uuid.UUID, model string) (EmbeddingModel, error) {
	m, err := s.models.EmbeddingModel(ctx, providerID, model)
	switch {
	case err != nil:
		return EmbeddingModel{}, err
	case !m.OpenAICompatible:
		return EmbeddingModel{}, &EmbeddingModelInvalidError{Reason: EmbeddingProviderKind}
	case !m.Embedding:
		return EmbeddingModel{}, &EmbeddingModelInvalidError{Reason: EmbeddingNotEmbedding}
	case m.InputPriceMicros == nil:
		return EmbeddingModel{}, &EmbeddingModelInvalidError{Reason: EmbeddingUnpriced}
	}
	return m, nil
}

func (s *Service) embedder(m EmbeddingModel, model string, dims int) (Embedder, error) {
	e, err := s.newEmbedder(embed.Config{
		BaseURL: m.BaseURL, APIKey: m.APIKey, Model: model, Dims: dims, AllowPrivateHosts: s.allowPrivateHosts,
	})
	if err != nil {
		return nil, EmbedError(err)
	}
	return e, nil
}
