//go:build integration

package knowledge_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
)

type fakeModels map[string]knowledge.EmbeddingModel

func (f fakeModels) EmbeddingModel(_ context.Context, _ uuid.UUID, model string) (knowledge.EmbeddingModel, error) {
	m, ok := f[model]
	if !ok {
		return knowledge.EmbeddingModel{}, apperror.New(apperror.CodeModelProviderUnknownModel, "unknown model")
	}
	return m, nil
}

func (fakeModels) LockProvider(ctx context.Context, _ uuid.UUID) error {
	if _, ok := db.CurrentTx(ctx); !ok {
		return errors.New("LockProvider outside a transaction")
	}
	return nil
}

type changingModels struct {
	fakeModels
	locked *bool
}

func (c changingModels) LockProvider(ctx context.Context, id uuid.UUID) error {
	*c.locked = true
	return c.fakeModels.LockProvider(ctx, id)
}

func (c changingModels) EmbeddingModel(ctx context.Context, id uuid.UUID, model string) (knowledge.EmbeddingModel, error) {
	m, err := c.fakeModels.EmbeddingModel(ctx, id, model)
	if *c.locked {
		m.Embedding = false
	}
	return m, err
}

func TestSetEmbeddingModel_RechecksUnderTheProviderLock(t *testing.T) {
	e := newEnv(t)
	locked := false
	svc := e.withEmbedding(1536, nil, embeddingModels).WithModels(changingModels{fakeModels: embeddingModels, locked: &locked})
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "text-embedding-3-small")
	assert.Equal(t, apperror.CodeKnowledgeEmbeddingModelInvalid, codeOf(err), "the model changed while it was probed")
	_, err = svc.Settings(e.ctx, e.prj)
	assert.True(t, knowledge.IsEmbeddingModelNotSetError(err), "nothing saved")
}

type fakeEmbedder struct {
	dims  int
	err   error
	calls *[]embed.Config
}

func (f fakeEmbedder) Probe(context.Context) (int, error) { return f.dims, f.err }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) (embed.Result, error) {
	if f.err != nil {
		return embed.Result{}, f.err
	}
	out := embed.Result{Dims: f.dims, Tokens: int64(len(texts)) * 10}
	for range texts {
		v := make([]float32, f.dims)
		v[0] = 1
		out.Vectors = append(out.Vectors, v)
	}
	return out, nil
}

func price(n int64) *int64 { return new(n) }

func (e *env) provider(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO model_providers (id, public_id, org_id, project_id, name, kind, base_url, api_key_sealed, models, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $2, 'openai_compatible', 'https://e.com/v1', '\x01', '[]', now(), now())`, id, "r"+id.String()[:12], e.org, e.prj)
	require.NoError(t, err)
	return id
}

func (e *env) withEmbedding(dims int, perr error, models fakeModels) *knowledge.Service {
	seen := &[]embed.Config{}
	return e.svc.WithModels(models).WithEmbedders(func(cfg embed.Config) (knowledge.Embedder, error) {
		*seen = append(*seen, cfg)
		return fakeEmbedder{dims: dims, err: perr, calls: seen}, nil
	})
}

var embeddingModels = fakeModels{
	"text-embedding-3-small": {OpenAICompatible: true, BaseURL: "https://e.com/v1", APIKey: "k", Embedding: true, InputPriceMicros: price(20_000)},
	"chat":                   {OpenAICompatible: true, BaseURL: "https://e.com/v1", APIKey: "k", InputPriceMicros: price(1)},
	"unpriced":               {OpenAICompatible: true, BaseURL: "https://e.com/v1", APIKey: "k", Embedding: true},
	"claude":                 {BaseURL: "https://api.anthropic.com", APIKey: "k", Embedding: true, InputPriceMicros: price(1)},
}

func TestSetEmbeddingModel_ProbeStoresDims(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(1536, nil, embeddingModels)
	p := e.provider(t)

	_, err := svc.Settings(e.ctx, e.prj)
	assert.True(t, knowledge.IsEmbeddingModelNotSetError(err))

	st, err := svc.SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	assert.Equal(t, 1536, st.Dims)
	got, err := svc.Settings(e.ctx, e.prj)
	require.NoError(t, err)
	assert.Equal(t, st.ModelKey(), got.ModelKey())
	assert.Equal(t, p, got.ProviderID)

	used, err := svc.EmbeddingModelOf(e.ctx, p)
	require.NoError(t, err)
	assert.Equal(t, "text-embedding-3-small", used)
	none, err := svc.EmbeddingModelOf(e.ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestEmbed_UnsupportedSizeRefused(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(384, nil, embeddingModels)
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "text-embedding-3-small")
	var inv *knowledge.EmbeddingModelInvalidError
	require.ErrorAs(t, err, &inv)
	assert.Equal(t, knowledge.EmbeddingSize, inv.Reason)
	assert.Equal(t, 384, inv.Dims)
	_, err = svc.Settings(e.ctx, e.prj)
	assert.True(t, knowledge.IsEmbeddingModelNotSetError(err), "nothing stored")
}

func TestSetEmbeddingModel_UnpricedRefused(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(1536, nil, embeddingModels)
	p := e.provider(t)
	for model, want := range map[string]knowledge.EmbeddingReason{
		"unpriced": knowledge.EmbeddingUnpriced,
		"chat":     knowledge.EmbeddingNotEmbedding,
	} {
		_, err := svc.SetEmbeddingModel(e.ctx, e.prj, p, model)
		var inv *knowledge.EmbeddingModelInvalidError
		require.ErrorAs(t, err, &inv, model)
		assert.Equal(t, want, inv.Reason, model)
	}
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, p, "nope")
	assert.Equal(t, apperror.CodeModelProviderUnknownModel, codeOf(err), "the provider's own code passes through")
}

func TestSetEmbeddingModel_AnthropicRefused(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(1536, nil, embeddingModels)
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "claude")
	var inv *knowledge.EmbeddingModelInvalidError
	require.ErrorAs(t, err, &inv)
	assert.Equal(t, knowledge.EmbeddingProviderKind, inv.Reason)
}

func TestSetEmbeddingModel_ProbeFailureIsEmbeddingFailed(t *testing.T) {
	e := newEnv(t)
	for reason, want := range map[embed.Reason]knowledge.EmbedFailReason{
		embed.Unreachable: knowledge.EmbeddingUnreachable,
		embed.Rejected:    knowledge.EmbeddingRejected,
		embed.WrongDims:   knowledge.EmbeddingWrongDims,
	} {
		svc := e.withEmbedding(0, &embed.Error{Reason: reason, Cause: errors.New("x")}, embeddingModels)
		_, err := svc.SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "text-embedding-3-small")
		var fail *knowledge.EmbeddingFailedError
		require.ErrorAs(t, err, &fail, reason)
		assert.Equal(t, want, fail.Reason)
		assert.Equal(t, apperror.CodeKnowledgeEmbeddingFailed, codeOf(err))
	}
}

func TestEmbeddingModelChange_RequeuesEveryDocument(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	p := e.provider(t)
	status := func(s string) uuid.UUID {
		id := uuid.New()
		_, err := e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 't', $6, now(), now())`, id, "d"+id.String()[:12], e.org, e.prj, src.ID, s)
		require.NoError(t, err)
		return id
	}
	ready, processing, failed := status("ready"), status("processing"), status("failed")
	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET lease_owner = 'w', lease_until = now() + interval '1 minute' WHERE id = $1`, processing)
	require.NoError(t, err)

	svc := e.withEmbedding(1536, nil, embeddingModels)
	_, err = svc.SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	for id, want := range map[uuid.UUID]knowledge.DocumentStatus{ready: knowledge.DocumentPending, processing: knowledge.DocumentPending, failed: knowledge.DocumentFailed} {
		d, err := svc.GetDocument(e.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, d.Status)
	}

	_, err = e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'ready' WHERE id = $1`, ready)
	require.NoError(t, err)
	_, err = svc.SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	d, err := svc.GetDocument(e.ctx, ready)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, d.Status, "setting the same model again re-embeds nothing")

	_, err = e.withEmbedding(3072, nil, embeddingModels).SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	d, err = svc.GetDocument(e.ctx, ready)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, d.Status, "a new size is a new model key")
}

func TestSetEmbeddingModel_ThroughTheGuardedClient(t *testing.T) {
	e := newEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "Bearer k", r.Header.Get("Authorization"))
		vec := make([]float32, 768)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vec}}, "usage": map[string]any{"prompt_tokens": 3, "total_tokens": 3}})
	}))
	t.Cleanup(srv.Close)
	models := fakeModels{"m": {OpenAICompatible: true, BaseURL: srv.URL + "/v1", APIKey: "k", Embedding: true, InputPriceMicros: price(1)}}

	_, err := e.svc.WithModels(models).SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "m")
	assert.True(t, knowledge.IsEmbeddingFailedError(err), "a loopback endpoint is refused by default: %v", err)

	st, err := e.svc.WithModels(models).WithAllowPrivateHosts(true).SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "m")
	require.NoError(t, err)
	assert.Equal(t, 768, st.Dims)
}
