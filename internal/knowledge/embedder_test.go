package knowledge_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
	"altalune.id/oagent/internal/knowledge/knowledgetest"
)

type oneModel struct {
	m   knowledge.EmbeddingModel
	err error
}

func (o oneModel) EmbeddingModel(context.Context, uuid.UUID, string) (knowledge.EmbeddingModel, error) {
	return o.m, o.err
}

func (oneModel) LockProvider(context.Context, uuid.UUID) error { return nil }

func TestEmbedderFor_BuildsTheProjectsClient(t *testing.T) {
	price := int64(20_000)
	model := knowledge.EmbeddingModel{OpenAICompatible: true, BaseURL: "https://llm.example.com/v1", APIKey: "sk-test", Embedding: true, InputPriceMicros: &price}
	st := &knowledge.Settings{ProviderID: uuid.New(), Model: "text-embedding-3-small", Dims: 1536}
	var built []embed.Config
	newEmbedder := func(cfg embed.Config) (knowledge.Embedder, error) {
		built = append(built, cfg)
		if cfg.Model == "refused" {
			return nil, &embed.Error{Reason: embed.Rejected}
		}
		return &knowledgetest.Embedder{Dims: cfg.Dims}, nil
	}
	svc := func(m knowledge.Models) *knowledge.Service {
		return knowledge.NewService(nil, nil, nil, nil, nil, nil).WithModels(m).WithEmbedders(newEmbedder).WithAllowPrivateHosts(true)
	}

	e, err := svc(oneModel{m: model}).EmbedderFor(t.Context(), st)
	require.NoError(t, err)
	dims, err := e.Probe(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1536, dims)
	assert.Equal(t, []embed.Config{{BaseURL: model.BaseURL, APIKey: "sk-test", Model: st.Model, Dims: 1536, AllowPrivateHosts: true}}, built)

	boom := errors.New("provider gone")
	_, err = svc(oneModel{err: boom}).EmbedderFor(t.Context(), st)
	require.ErrorIs(t, err, boom, "the provider's own error passes through")

	unpriced := model
	unpriced.InputPriceMicros = nil
	_, err = svc(oneModel{m: unpriced}).EmbedderFor(t.Context(), st)
	require.True(t, knowledge.IsEmbeddingModelInvalidError(err), "%v", err)

	refused := *st
	refused.Model = "refused"
	_, err = svc(oneModel{m: model}).EmbedderFor(t.Context(), &refused)
	require.True(t, knowledge.IsEmbeddingFailedError(err), "%v", err)
}

type settingsOnly struct {
	knowledge.Store
	st  *knowledge.Settings
	err error
}

func (s settingsOnly) Settings(context.Context, uuid.UUID) (*knowledge.Settings, error) {
	return s.st, s.err
}

func TestEmbeddingPrice_IsTheModelsInputPrice(t *testing.T) {
	price := int64(20_000)
	st := &knowledge.Settings{ProviderID: uuid.New(), Model: "text-embedding-3-small", Dims: 1536}
	svc := func(store knowledge.Store, m knowledge.Models) *knowledge.Service {
		return knowledge.NewService(store, nil, nil, nil, nil, nil).WithModels(m)
	}

	got, err := svc(settingsOnly{st: st}, oneModel{m: knowledge.EmbeddingModel{InputPriceMicros: &price}}).EmbeddingPrice(t.Context(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, &price, got)

	got, err = svc(settingsOnly{err: &knowledge.EmbeddingModelNotSetError{}}, oneModel{}).EmbeddingPrice(t.Context(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, got, "a project with no embedding model has no price")

	boom := errors.New("db down")
	_, err = svc(settingsOnly{err: boom}, oneModel{}).EmbeddingPrice(t.Context(), uuid.New())
	require.ErrorIs(t, err, boom)
	_, err = svc(settingsOnly{st: st}, oneModel{err: boom}).EmbeddingPrice(t.Context(), uuid.New())
	require.ErrorIs(t, err, boom)
}

func TestEmbedError_MapsTheClientsReasons(t *testing.T) {
	for in, want := range map[embed.Reason]knowledge.EmbedFailReason{
		embed.Unreachable: knowledge.EmbeddingUnreachable,
		embed.Rejected:    knowledge.EmbeddingRejected,
		embed.WrongDims:   knowledge.EmbeddingWrongDims,
	} {
		err := knowledge.EmbedError(fmt.Errorf("wrapped: %w", &embed.Error{Reason: in}))
		e, ok := errors.AsType[*knowledge.EmbeddingFailedError](err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, want, e.Reason)
	}
	boom := errors.New("not an embed error")
	assert.Same(t, boom, knowledge.EmbedError(boom), "anything else passes through")
}
