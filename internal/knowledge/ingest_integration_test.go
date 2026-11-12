//go:build integration

package knowledge_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/embed"
)

type oneTenant struct{ ctx context.Context }

func (o oneTenant) Each(_ context.Context, fn func(context.Context, string) error) error {
	return fn(o.ctx, "")
}

type rounder interface {
	Round(ctx context.Context)
	Wait()
}

func runRound(t *testing.T, w rounder) {
	t.Helper()
	w.Round(t.Context())
	w.Wait()
}

type midRunChange struct {
	fakeEmbedder
	change func()
}

func (m midRunChange) Embed(ctx context.Context, texts []string) (embed.Result, error) {
	m.change()
	return m.fakeEmbedder.Embed(ctx, texts)
}

type mismatchEmbedder struct{ fakeEmbedder }

func (mismatchEmbedder) Embed(context.Context, []string) (embed.Result, error) {
	return embed.Result{}, &embed.Error{Reason: embed.WrongDims, Cause: errors.New("vector of 3, want 1536")}
}

func (e *env) ingesting(t *testing.T, dims int) (*knowledge.Service, uuid.UUID) {
	t.Helper()
	svc := e.withEmbedding(dims, nil, embeddingModels).WithExtractor(childExtractor())
	p := e.provider(t)
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	return svc, p
}

func (e *env) uploaded(t *testing.T, src *knowledge.Source, body string) *knowledge.Document {
	t.Helper()
	doc, _, err := e.svc.BeginUpload(e.ctx, src.ID, knowledge.UploadInput{Filename: "faq.md", ContentType: knowledge.TypeMarkdown, Size: int64(len(body))})
	require.NoError(t, err)
	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypeMarkdown, []byte(body))
	doc, err = e.svc.CompleteUpload(e.ctx, doc.ID)
	require.NoError(t, err)
	return doc
}

func (e *env) chunkRows(t *testing.T, doc uuid.UUID) (n int, keys []string) {
	t.Helper()
	rows, err := e.admin.QueryContext(t.Context(), `SELECT DISTINCT model_key FROM knowledge_chunks WHERE document_id = $1`, doc)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		keys = append(keys, k)
	}
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM knowledge_chunks WHERE document_id = $1`, doc).Scan(&n))
	return n, keys
}

const faq = "# Refunds\n\nWe refund within 30 days.\n\n# Shipping\n\nWe ship worldwide in 5 days.\n"

func TestIngest_ChunksEmbedsAndRecordsCost(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 1536)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)

	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, got.Status)
	n, keys := e.chunkRows(t, doc.ID)
	assert.Positive(t, n)
	st, err := svc.Settings(e.ctx, e.prj)
	require.NoError(t, err)
	assert.Equal(t, []string{st.ModelKey()}, keys)
	assert.Equal(t, int64(n)*10, got.EmbedTokens, "the provider's token count")
	assert.Equal(t, got.EmbedTokens*20_000/1_000_000, got.EmbedCostMicros, "priced at the ingest-time input price")
}

func TestIngest_RerunIdempotent(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	w := knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler))
	runRound(t, w)
	first, _ := e.chunkRows(t, doc.ID)

	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'pending' WHERE id = $1`, doc.ID)
	require.NoError(t, err)
	runRound(t, w)
	second, _ := e.chunkRows(t, doc.ID)
	assert.Equal(t, first, second, "a re-run replaces the chunks, never adds to them")
}

func TestIngest_LeaseAndReconciler(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	src := e.uploadSource(t, "FAQ")
	for range 3 {
		e.uploaded(t, src, faq)
	}

	claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 4)
	require.NoError(t, err)
	assert.Len(t, claims, knowledge.MaxIngestPerProject, "at most two of a project at once")
	again, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 4)
	require.NoError(t, err)
	assert.Empty(t, again, "a live lease is not claimed twice, and the project is full")

	stale, err := e.store.ClaimDocuments(e.ctx, time.Now().Add(2*time.Minute), time.Minute, 4)
	require.NoError(t, err)
	assert.Len(t, stale, knowledge.MaxIngestPerProject, "the reconciler claims expired leases again")
	assert.NotEqual(t, claims[0].Token, stale[0].Token)

	require.NoError(t, svc.Ingest(e.ctx, claims[0]), "the old lease holder writes nothing")
	got, err := svc.GetDocument(e.ctx, claims[0].Document.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentProcessing, got.Status)
	n, _ := e.chunkRows(t, claims[0].Document.ID)
	assert.Zero(t, n)
}

func TestIngest_ModelChangedMidRunRollsBack(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)

	changing := midRunChange{fakeEmbedder: fakeEmbedder{dims: 768}, change: func() {
		_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_settings SET dims = 1024 WHERE project_id = $1`, e.prj)
		require.NoError(t, err, "the model changes while the run still holds its lease")
	}}
	require.NoError(t, svc.WithEmbedders(func(embed.Config) (knowledge.Embedder, error) { return changing, nil }).Ingest(e.ctx, claims[0]))

	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, got.Status, "a run started under the old model leaves the document for the new one")
	n, _ := e.chunkRows(t, doc.ID)
	assert.Zero(t, n, "no chunk of the old model is written")
}

func TestEmbeddingModelChange_ReembedsNoMixedModel(t *testing.T) {
	e := newEnv(t)
	svc, p := e.ingesting(t, 768)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	w := knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler))
	runRound(t, w)
	before, _ := e.chunkRows(t, doc.ID)

	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypeMarkdown, []byte("# Gone\n\nThe file changed, but a re-embed reads stored chunks.\n"))
	svc2 := e.withEmbedding(1536, nil, embeddingModels).WithExtractor(childExtractor())
	_, err := svc2.SetEmbeddingModel(e.ctx, e.prj, p, "text-embedding-3-small")
	require.NoError(t, err)
	runRound(t, knowledge.NewIngestWorker(svc2, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	n, keys := e.chunkRows(t, doc.ID)
	st, err := svc2.Settings(e.ctx, e.prj)
	require.NoError(t, err)
	assert.Equal(t, []string{st.ModelKey()}, keys, "every chunk carries the new model key")
	assert.Equal(t, before, n, "re-embedded from the stored chunk text, not re-extracted")
	var dims int
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT DISTINCT dims FROM knowledge_chunks WHERE document_id = $1`, doc.ID).Scan(&dims))
	assert.Equal(t, 1536, dims)
}

func TestEmbed_LengthMismatchFailsDocument(t *testing.T) {
	e := newEnv(t)
	e.ingesting(t, 1536)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	svc := e.svc.WithModels(embeddingModels).WithExtractor(childExtractor()).
		WithEmbedders(func(embed.Config) (knowledge.Embedder, error) { return mismatchEmbedder{fakeEmbedder{dims: 1536}}, nil })
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeEmbeddingFailed, Reason: "wrong_dims"}, got.Failure)
	n, _ := e.chunkRows(t, doc.ID)
	assert.Zero(t, n, "nothing goes under the wrong index")
}

func TestIngest_NoModelLeavesItPending(t *testing.T) {
	e := newEnv(t)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	svc := e.svc.WithExtractor(childExtractor())
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, got.Status, "it waits for an embedding model")
}

func TestIngest_FailuresAreCoded(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	src := e.uploadSource(t, "FAQ")
	empty := e.uploaded(t, src, "   \n\n")
	badUTF8 := e.uploaded(t, src, "caf\xe9")
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	for id, want := range map[uuid.UUID]knowledge.Failure{
		empty.ID:   {Code: apperror.CodeKnowledgeNoTextFound},
		badUTF8.ID: {Code: apperror.CodeKnowledgeUploadRejected, Reason: "invalid_utf8"},
	} {
		got, err := svc.GetDocument(e.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, knowledge.DocumentFailed, got.Status)
		assert.Equal(t, &want, got.Failure)
	}
}

func TestIngest_ProjectChunkCap(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 768)
	src := e.uploadSource(t, "FAQ")
	filler := e.uploaded(t, src, faq)
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_chunks (id, org_id, project_id, source_id, document_id, ordinal, text, embedding, dims, model_key, created_at)
		SELECT gen_random_uuid(), $1, $2, $3, $4, g, 't', array_fill(0.1::real, ARRAY[768])::public.vector, 768, 'old', now() FROM generate_series(1, $5) g`,
		e.org, e.prj, src.ID, filler.ID, knowledge.MaxChunksPerProject)
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'ready' WHERE id = $1`, filler.ID)
	require.NoError(t, err)
	doc := e.uploaded(t, src, faq)
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeLimitReached, Reason: "chunks_per_project"}, got.Failure)
}

func (e *env) ingestRound(t *testing.T, svc *knowledge.Service) {
	t.Helper()
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
}
