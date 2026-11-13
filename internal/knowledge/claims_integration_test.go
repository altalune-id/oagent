//go:build integration

package knowledge_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/embed"
)

func (e *env) projectIn(t *testing.T, org uuid.UUID) uuid.UUID {
	t.Helper()
	prj := uuid.New()
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $2, 'P', now(), now())`,
		prj, "p"+prj.String()[:12], org)
	require.NoError(t, err)
	return prj
}

func (e *env) withModel(t *testing.T, org, prj uuid.UUID) {
	t.Helper()
	p := uuid.New()
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO model_providers (id, public_id, org_id, project_id, name, kind, base_url, api_key_sealed, models, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $2, 'openai_compatible', 'https://e.com/v1', '\x01', '[]', now(), now())`, p, "r"+p.String()[:12], org, prj)
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_settings (project_id, org_id, provider_id, model, dims, updated_at)
		VALUES ($1, $2, $3, 'text-embedding-3-small', 768, now())`, prj, org, p)
	require.NoError(t, err)
}

func (e *env) pendingPages(t *testing.T, org, prj uuid.UUID, n int) []uuid.UUID {
	t.Helper()
	src := uuid.New()
	_, err := e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, seed_url, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'url', 'pgvector', $2, 'https://e.com', 'idle', now(), now())`, src, "s"+src.String()[:12], org, prj)
	require.NoError(t, err)
	ids := make([]uuid.UUID, n)
	base := time.Now().Add(-time.Hour).UTC()
	for i := range ids {
		ids[i] = uuid.New()
		_, err := e.admin.ExecContext(t.Context(), `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, url, content, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'T', $6, '[{"text":"hello"}]', 'pending', $7, $7)`,
			ids[i], "d"+ids[i].String()[:12], org, prj, src, fmt.Sprintf("https://e.com/%d", i), base.Add(time.Duration(i)*time.Second))
		require.NoError(t, err)
	}
	return ids
}

func projectsOf(claims []knowledge.Claim) map[uuid.UUID]int {
	out := map[uuid.UUID]int{}
	for _, c := range claims {
		out[c.Document.ProjectID]++
	}
	return out
}

// SECURITY: two claimers in one org never lease the same document or pass the per-project cap.
func TestClaimDocuments_OneClaimerPerOrgAtATime(t *testing.T) {
	e := newEnv(t)
	e.withModel(t, e.org, e.prj)
	e.pendingPages(t, e.org, e.prj, 10)

	var mu sync.Mutex
	var all []knowledge.Claim
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 4)
			assert.NoError(t, err)
			mu.Lock()
			all = append(all, claims...)
			mu.Unlock()
		})
	}
	wg.Wait()
	seen := map[uuid.UUID]bool{}
	for _, c := range all {
		assert.False(t, seen[c.Document.ID], "claimed twice")
		seen[c.Document.ID] = true
	}
	assert.Len(t, all, knowledge.MaxIngestPerProject, "the project cap holds across claimers")

	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = db.NewTransactor(e.pool).RunInTx(e.ctx, func(ctx context.Context) error {
			if err := db.LockInTx(ctx, "knowledge-ingest:"+e.org.String()); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer close(release)
	ctx, cancel := context.WithTimeout(e.ctx, 300*time.Millisecond)
	defer cancel()
	_, err := e.store.ClaimDocuments(ctx, time.Now().Add(time.Hour), time.Minute, 4)
	assert.Error(t, err, "a claimer waits for the org's claim lock")
}

func TestClaimDocuments_BusyProjectDoesNotStarveAnother(t *testing.T) {
	e := newEnv(t)
	quiet := e.projectIn(t, e.org)
	e.withModel(t, e.org, e.prj)
	e.withModel(t, e.org, quiet)
	e.pendingPages(t, e.org, e.prj, 70)
	newest := e.pendingPages(t, e.org, quiet, 1)[0]
	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET updated_at = now() WHERE id = $1`, newest)
	require.NoError(t, err)
	unset := e.projectIn(t, e.org)
	e.pendingPages(t, e.org, unset, 1)

	claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 2)
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]int{e.prj: 1, quiet: 1}, projectsOf(claims), "each project's oldest goes first")

	more, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 4)
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]int{e.prj: 1}, projectsOf(more), "a project without a model is never claimed")

	none, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 4)
	require.NoError(t, err)
	assert.Empty(t, none, "the busy project is at its cap while its leases live")
}

func TestClaimDocuments_FailsAfterMaxAttempts(t *testing.T) {
	e := newEnv(t)
	e.withModel(t, e.org, e.prj)
	id := e.pendingPages(t, e.org, e.prj, 1)[0]

	now := time.Now()
	for i := 1; i < knowledge.MaxIngestAttempts+1; i++ {
		claims, err := e.store.ClaimDocuments(e.ctx, now, time.Minute, 4)
		require.NoError(t, err)
		require.Len(t, claims, 1, "attempt %d", i)
		assert.Equal(t, i, claims[0].Document.Attempts)
		now = now.Add(2 * time.Minute)
	}
	claims, err := e.store.ClaimDocuments(e.ctx, now, time.Minute, 4)
	require.NoError(t, err)
	assert.Empty(t, claims)
	got, err := e.svc.GetDocument(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, got.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeCouldNotReadFile, Reason: "crashed"}, got.Failure)
}

func TestRenewLease_OnlyTheHolder(t *testing.T) {
	e := newEnv(t)
	e.withModel(t, e.org, e.prj)
	e.pendingPages(t, e.org, e.prj, 1)
	claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	c := claims[0]
	require.NoError(t, e.store.RenewLease(e.ctx, c, time.Now().Add(time.Hour)))
	again, err := e.store.ClaimDocuments(e.ctx, time.Now().Add(30*time.Minute), time.Minute, 1)
	require.NoError(t, err)
	assert.Empty(t, again, "a renewed lease is not claimed again")
	assert.Error(t, e.store.RenewLease(e.ctx, knowledge.Claim{Document: c.Document, Token: "stolen"}, time.Now().Add(time.Hour)))
	assert.Error(t, e.store.RenewLease(e.other, c, time.Now().Add(time.Hour)), "another org renews nothing")
}

type stealingCrawler struct {
	steal func()
}

func (s stealingCrawler) Crawl(_ context.Context, _ string, visit func(crawl.Page) error) ([]string, error) {
	s.steal()
	err := visit(html("https://help.example.com/", "<h1>Help</h1><p>a</p>"))
	return []string{"https://help.example.com/"}, err
}

// SECURITY: a crawl that lost its lease writes and deletes nothing, and leaves the new holder's crawl running.
func TestCrawl_LostLeaseWritesNothing(t *testing.T) {
	e := newEnv(t)
	first := e.crawlWith(fakeCrawler{pages: []crawl.Page{html("https://help.example.com/old", "<h1>Old</h1><p>b</p>")}})
	src := e.urlSource(t, first)
	runRound(t, knowledge.NewCrawlWorker(first, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
	_, err := first.Refresh(e.ctx, src.ID)
	require.NoError(t, err)

	svc := e.svc.WithCrawler(func() knowledge.Crawler {
		return stealingCrawler{steal: func() {
			_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_sources SET lease_owner = 'thief' WHERE id = $1`, src.ID)
			require.NoError(t, err)
		}}
	})
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	docs, err := svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "https://help.example.com/old", docs[0].URL, "no new page and no deleted page")
	got, err := svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceCrawling, got.Status, "the thief's crawl is not ended")
}

func TestCrawl_UnchangedReadyPageStaysReady(t *testing.T) {
	e := newEnv(t)
	page := html("https://help.example.com/", "<h1>Help</h1><p>Start here.</p>")
	svc, _ := e.ingesting(t, 768)
	svc = svc.WithCrawler(func() knowledge.Crawler { return fakeCrawler{pages: []crawl.Page{page}} })
	src := e.urlSource(t, svc)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
	e.ingestRound(t, svc)
	docs, err := svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, knowledge.DocumentReady, docs[0].Status)
	var content *string
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT content::text FROM knowledge_documents WHERE id = $1`, docs[0].ID).Scan(&content))
	assert.Nil(t, content, "a ready document keeps no content")

	_, err = svc.Refresh(e.ctx, src.ID)
	require.NoError(t, err)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
	again, err := svc.GetDocument(e.ctx, docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, again.Status, "an unchanged page is not embedded again")

	svc = svc.WithCrawler(func() knowledge.Crawler {
		return fakeCrawler{pages: []crawl.Page{html("https://help.example.com/", "<h1>Help</h1><p>Changed.</p>")}}
	})
	e.refreshable(t, src.ID)
	_, err = svc.Refresh(e.ctx, src.ID)
	require.NoError(t, err)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
	changed, err := svc.GetDocument(e.ctx, docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, changed.Status, "a changed page is queued")
}

func TestRefresh_DuringACrawlRunsAgain(t *testing.T) {
	e := newEnv(t)
	src := e.urlSource(t, e.svc)
	c, err := e.store.ClaimSource(e.ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, c)
	_, err = e.svc.Refresh(e.ctx, src.ID)
	require.NoError(t, err)
	require.NoError(t, e.store.FinishCrawl(e.ctx, *c, nil, time.Now()))
	got, err := e.svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceCrawling, got.Status, "the refresh asked for mid-crawl is kept")
	next, err := e.store.ClaimSource(e.ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.NoError(t, e.store.FinishCrawl(e.ctx, *next, nil, time.Now()))
	got, err = e.svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceIdle, got.Status)
}

type twoOrgs struct{ a, b context.Context }

func (o twoOrgs) Each(_ context.Context, fn func(context.Context, string) error) error {
	if err := fn(o.a, ""); err != nil {
		return err
	}
	return fn(o.b, "")
}

type gatedEmbedder struct {
	fakeEmbedder
	gate    chan struct{}
	blocked uuid.UUID
}

func (g gatedEmbedder) Embed(ctx context.Context, texts []string) (embed.Result, error) {
	if tenant.Peek(ctx).OrgID == g.blocked {
		select {
		case <-g.gate:
		case <-ctx.Done():
			return embed.Result{}, ctx.Err()
		}
	}
	return g.fakeEmbedder.Embed(ctx, texts)
}

func TestIngestWorker_SlowOrgDoesNotBlockAnother(t *testing.T) {
	e := newEnv(t)
	gate := make(chan struct{})
	svc := e.svc.WithModels(embeddingModels).WithExtractor(childExtractor()).
		WithEmbedders(func(embed.Config) (knowledge.Embedder, error) {
			return gatedEmbedder{fakeEmbedder: fakeEmbedder{dims: 768}, gate: gate, blocked: e.org}, nil
		})
	e.withModel(t, e.org, e.prj)
	e.withModel(t, e.otherOrg, e.otherPrj)
	slow := e.pendingPages(t, e.org, e.prj, 1)[0]
	fast := e.pendingPages(t, e.otherOrg, e.otherPrj, 1)[0]

	w := knowledge.NewIngestWorker(svc, twoOrgs{e.ctx, e.other}, slog.New(slog.DiscardHandler))
	w.Round(t.Context())
	require.Eventually(t, func() bool {
		d, err := svc.GetDocument(e.other, fast)
		return err == nil && d.Status == knowledge.DocumentReady
	}, 10*time.Second, 20*time.Millisecond, "org B ingests while org A's document is stuck")
	d, err := svc.GetDocument(e.ctx, slow)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentProcessing, d.Status)
	close(gate)
	w.Wait()
	d, err = svc.GetDocument(e.ctx, slow)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, d.Status)
}

func TestRequeueDocuments_RetriesEmbeddingFailures(t *testing.T) {
	e := newEnv(t)
	ids := e.pendingPages(t, e.org, e.prj, 3)
	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'failed', error_code = $2, error_reason = 'unreachable', attempts = 3 WHERE id = $1`,
		ids[0], string(apperror.CodeKnowledgeEmbeddingFailed))
	require.NoError(t, err)
	_, err = e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'failed', error_code = $2 WHERE id = $1`,
		ids[1], string(apperror.CodeKnowledgeNoTextFound))
	require.NoError(t, err)
	otherIDs := e.pendingPages(t, e.otherOrg, e.otherPrj, 1)
	_, err = e.admin.ExecContext(t.Context(), `UPDATE knowledge_documents SET status = 'ready' WHERE id = $1`, otherIDs[0])
	require.NoError(t, err)

	require.NoError(t, e.store.RequeueDocuments(e.ctx, e.prj))
	got, err := e.svc.GetDocument(e.ctx, ids[0])
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentPending, got.Status, "a failed embedding is tried again with the new model")
	assert.Nil(t, got.Failure)
	assert.Zero(t, got.Attempts)
	kept, err := e.svc.GetDocument(e.ctx, ids[1])
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentFailed, kept.Status, "a file with no text stays failed")
	other, err := e.svc.GetDocument(e.other, otherIDs[0])
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, other.Status, "another org's documents are untouched")
}

func (e *env) refreshable(t *testing.T, sourceID uuid.UUID) {
	t.Helper()
	_, err := e.admin.ExecContext(t.Context(), `UPDATE knowledge_sources SET refreshed_at = now() - interval '11 minutes' WHERE id = $1`, sourceID)
	require.NoError(t, err)
}

// SECURITY: a re-crawl of an unchanged page leaves an ingest in flight alone, so its lease and paid work survive.
func TestCrawl_UnchangedPageKeepsTheIngestLease(t *testing.T) {
	e := newEnv(t)
	page := html("https://help.example.com/", "<h1>Help</h1><p>Start here.</p>")
	svc, _ := e.ingesting(t, 768)
	svc = svc.WithCrawler(func() knowledge.Crawler { return fakeCrawler{pages: []crawl.Page{page}} })
	src := e.urlSource(t, svc)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	claims, err := e.store.ClaimDocuments(e.ctx, time.Now(), time.Hour, 4)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	e.refreshable(t, src.ID)
	_, err = svc.Refresh(e.ctx, src.ID)
	require.NoError(t, err)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	var status string
	var owner *string
	var attempts int
	require.NoError(t, e.admin.QueryRowContext(t.Context(), `SELECT status, lease_owner, attempts FROM knowledge_documents WHERE id = $1`,
		claims[0].Document.ID).Scan(&status, &owner, &attempts))
	assert.Equal(t, "processing", status)
	require.NotNil(t, owner)
	assert.Equal(t, claims[0].Token, *owner, "the ingest keeps its lease")
	assert.Equal(t, 1, attempts, "and its attempt count")
	require.NoError(t, e.store.RenewLease(e.ctx, claims[0], time.Now().Add(time.Hour)), "the holder can still renew")
}
