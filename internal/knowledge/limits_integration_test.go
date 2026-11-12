//go:build integration

package knowledge_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/embed"
)

type countingEmbedder struct {
	fakeEmbedder
	embeds *atomic.Int32
	result func([]string) (embed.Result, error)
}

func (c countingEmbedder) Embed(ctx context.Context, texts []string) (embed.Result, error) {
	c.embeds.Add(1)
	if c.result != nil {
		return c.result(texts)
	}
	return c.fakeEmbedder.Embed(ctx, texts)
}

func (e *env) counting(t *testing.T, dims int, result func([]string) (embed.Result, error)) (*knowledge.Service, *atomic.Int32) {
	t.Helper()
	svc, _ := e.ingesting(t, 768)
	n := &atomic.Int32{}
	svc.WithEmbedders(func(embed.Config) (knowledge.Embedder, error) {
		return countingEmbedder{fakeEmbedder: fakeEmbedder{dims: dims}, embeds: n, result: result}, nil
	})
	return svc, n
}

func (e *env) document(t *testing.T, svc *knowledge.Service, id uuid.UUID) *knowledge.Document {
	t.Helper()
	d, err := svc.GetDocument(e.ctx, id)
	require.NoError(t, err)
	return d
}

func TestIngest_ProjectChunkCapCheckedBeforeEmbedding(t *testing.T) {
	e := newEnv(t)
	svc, embeds := e.counting(t, 768, nil)
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

	got := e.document(t, svc, doc.ID)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeLimitReached, Reason: "chunks_per_project"}, got.Failure)
	assert.Zero(t, embeds.Load(), "a document over the cap is never paid for")
	assert.Zero(t, got.EmbedTokens)
}

func TestIngest_FailedEmbeddingStillRecordsItsSpend(t *testing.T) {
	e := newEnv(t)
	svc, embeds := e.counting(t, 768, func([]string) (embed.Result, error) {
		return embed.Result{Tokens: 50}, &embed.Error{Reason: embed.Unreachable, Cause: errors.New("reset mid-batch")}
	})
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got := e.document(t, svc, doc.ID)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
	assert.Equal(t, apperror.CodeKnowledgeEmbeddingFailed, got.Failure.Code)
	assert.Equal(t, int64(50), got.EmbedTokens, "tokens billed before the failure are recorded")
	assert.Equal(t, int64(50)*20_000/1_000_000, got.EmbedCostMicros)
	assert.Equal(t, int32(1), embeds.Load())
}

// SECURITY: a failure after the provider was paid ends the document instead of paying again for the same text.
func TestIngest_FailureAfterEmbeddingIsNotRetried(t *testing.T) {
	e := newEnv(t)
	svc, embeds := e.counting(t, 1024, nil)
	doc := e.uploaded(t, e.uploadSource(t, "FAQ"), faq)
	w := knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler))
	runRound(t, w)

	got := e.document(t, svc, doc.ID)
	require.Equal(t, knowledge.DocumentFailed, got.Status, "the 1024-wide vectors break the 768 insert")
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeCouldNotReadFile, Reason: "crashed"}, got.Failure)
	assert.Positive(t, got.EmbedTokens, "the paid attempt is recorded")
	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET lease_until = NULL WHERE id = $1`, doc.ID)
	require.NoError(t, err)
	runRound(t, w)
	assert.Equal(t, int32(1), embeds.Load(), "the provider is never paid twice for it")
}

func TestCreateSource_ProjectSourceCap(t *testing.T) {
	e := newEnv(t)
	for i := range knowledge.MaxSourcesPerProject {
		_, err := e.svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: fmt.Sprintf("S%d", i)})
		require.NoError(t, err)
	}
	_, err := e.svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "one too many"})
	var lim *knowledge.LimitReachedError
	require.ErrorAs(t, err, &lim)
	assert.Equal(t, knowledge.LimitReachedError{Reason: knowledge.LimitSourcesPerProject, Max: knowledge.MaxSourcesPerProject}, *lim)
	_, err = e.svc.CreateSource(e.other, e.otherPrj, knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "theirs"})
	assert.NoError(t, err, "the cap is per project")
}

func TestRefresh_MinimumInterval(t *testing.T) {
	e := newEnv(t)
	src := e.urlSource(t, e.svc)
	_, err := e.svc.Refresh(e.ctx, src.ID)
	require.NoError(t, err, "the first refresh goes through")
	_, err = e.svc.Refresh(e.ctx, src.ID)
	var lim *knowledge.LimitReachedError
	require.ErrorAs(t, err, &lim)
	assert.Equal(t, knowledge.LimitReachedError{Reason: knowledge.LimitRefreshTooSoon, Max: 10}, *lim)

	_, err = e.admin.ExecContext(t.Context(), `UPDATE knowledge_sources SET refreshed_at = now() - interval '11 minutes' WHERE id = $1`, src.ID)
	require.NoError(t, err)
	_, err = e.svc.Refresh(e.ctx, src.ID)
	assert.NoError(t, err, "after the interval it goes through")
	_, err = e.svc.Refresh(e.ctx, uuid.New())
	assert.True(t, knowledge.IsSourceNotFoundError(err))
}

type blockingCrawler struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (b blockingCrawler) Crawl(ctx context.Context, _ string, _ func(crawl.Page) error) ([]string, error) {
	b.started <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil, nil
}

type orgsEach []context.Context

func (o orgsEach) Each(_ context.Context, fn func(context.Context, string) error) error {
	for _, ctx := range o {
		if err := fn(ctx, ""); err != nil {
			return err
		}
	}
	return nil
}

func (e *env) leasedSources(t *testing.T, org uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM knowledge_sources WHERE org_id = $1 AND lease_owner IS NOT NULL`, org).Scan(&n))
	return n
}

func TestCrawlWorker_OneOrgTakesAtMostTwoSlots(t *testing.T) {
	e := newEnv(t)
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	svc := e.svc.WithCrawler(func() knowledge.Crawler { return blockingCrawler{started: started, release: release} }).WithExtractor(childExtractor())
	for i := range 3 {
		_, err := svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: fmt.Sprintf("A%d", i), SeedURL: "https://a.example.com"})
		require.NoError(t, err)
	}
	_, err := svc.CreateSource(e.other, e.otherPrj, knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "B", SeedURL: "https://b.example.com"})
	require.NoError(t, err)

	w := knowledge.NewCrawlWorker(svc, orgsEach{e.ctx, e.other}, slog.New(slog.DiscardHandler))
	defer w.Wait()
	defer close(release)
	for range 3 {
		w.Round(t.Context())
	}
	for range 3 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("a crawl never started")
		}
	}
	assert.Equal(t, knowledge.MaxCrawlPerOrg, e.leasedSources(t, e.org), "org A holds at most two of the slots")
	assert.Equal(t, 1, e.leasedSources(t, e.otherOrg), "org B still gets one")
}
