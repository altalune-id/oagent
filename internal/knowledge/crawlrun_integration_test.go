//go:build integration

package knowledge_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/crawl"
)

type fakeCrawler struct {
	pages []crawl.Page
	err   error
}

func (f fakeCrawler) Crawl(_ context.Context, _ string, visit func(crawl.Page) error) ([]string, error) {
	var visited []string
	for _, p := range f.pages {
		visited = append(visited, p.URL)
		if err := visit(p); err != nil {
			return visited, err
		}
	}
	return visited, f.err
}

func html(url, body string) crawl.Page {
	return crawl.Page{URL: url, ContentType: "text/html; charset=utf-8", Body: []byte(body)}
}

func (e *env) urlSource(t *testing.T, svc *knowledge.Service) *knowledge.Source {
	t.Helper()
	src, err := svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: "Help", SeedURL: "https://help.example.com"})
	require.NoError(t, err)
	return src
}

func (e *env) crawlWith(c fakeCrawler) *knowledge.Service {
	return e.svc.WithCrawler(func() knowledge.Crawler { return c }).WithExtractor(childExtractor())
}

func TestCrawl_StoresPagesAsPendingDocuments(t *testing.T) {
	e := newEnv(t)
	svc := e.crawlWith(fakeCrawler{pages: []crawl.Page{
		html("https://help.example.com/", "<html><body><h1>Help</h1><p>Start here.</p></body></html>"),
		{URL: "https://help.example.com/logo.png", ContentType: "image/png", Body: []byte{1}},
		html("https://help.example.com/refunds", "<h1>Refunds</h1><p>Thirty days.</p>"),
	}})
	crawls := 0
	svc.WithCrawlKick(func() { crawls++ })
	src := e.urlSource(t, svc)
	assert.Equal(t, 1, crawls, "a new URL source wakes the crawl worker")
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceIdle, got.Status)
	docs, err := svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, docs, 2, "a page with no text is not a document")
	assert.Equal(t, "Help", docs[0].Title)
	assert.Equal(t, knowledge.DocumentPending, docs[0].Status)
	assert.Equal(t, "https://help.example.com/refunds", docs[1].URL)
}

func TestRefresh_DeletesVanishedPages(t *testing.T) {
	e := newEnv(t)
	first := e.crawlWith(fakeCrawler{pages: []crawl.Page{
		html("https://help.example.com/", "<h1>Help</h1><p>a</p>"), html("https://help.example.com/old", "<h1>Old</h1><p>b</p>"),
	}})
	src := e.urlSource(t, first)
	runRound(t, knowledge.NewCrawlWorker(first, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	second := e.crawlWith(fakeCrawler{pages: []crawl.Page{html("https://help.example.com/", "<h1>Help v2</h1><p>a</p>")}})
	refreshed, err := second.Refresh(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceCrawling, refreshed.Status)
	runRound(t, knowledge.NewCrawlWorker(second, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	docs, err := second.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1, "the vanished page is deleted with its chunks")
	assert.Equal(t, "Help v2", docs[0].Title, "a page seen again is updated in place")

	up := e.uploadSource(t, "Files")
	_, err = second.Refresh(e.ctx, up.ID)
	assert.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "an upload source has nothing to crawl")
}

func TestCrawl_OffHostSeedRedirectExplained(t *testing.T) {
	e := newEnv(t)
	for name, tc := range map[string]struct {
		crawler fakeCrawler
		want    string
	}{
		"off-host seed": {fakeCrawler{err: &crawl.OffHostRedirectError{Host: "www.example.com"}}, "off_host_redirect"},
		"unreachable":   {fakeCrawler{err: errors.New("dial tcp: refused")}, "unreachable"},
		"no pages":      {fakeCrawler{}, "no_pages"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := e.crawlWith(tc.crawler)
			src, err := svc.CreateSource(e.ctx, e.prj, knowledge.SourceInput{Kind: knowledge.KindURL, Backend: knowledge.BackendPgvector, Name: name, SeedURL: "https://e.com"})
			require.NoError(t, err)
			runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))
			got, err := svc.GetSource(e.ctx, src.ID)
			require.NoError(t, err)
			assert.Equal(t, knowledge.SourceFailed, got.Status)
			assert.Equal(t, &knowledge.Failure{Code: apperror.CodeKnowledgeCrawlFailed, Reason: tc.want}, got.Failure)
		})
	}
}

func TestCrawl_SourceLeaseAndReconciler(t *testing.T) {
	e := newEnv(t)
	src := e.urlSource(t, e.svc)
	c, err := e.store.ClaimSource(e.ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, src.ID, c.Source.ID)
	again, err := e.store.ClaimSource(e.ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	assert.Nil(t, again, "a live crawl lease is not claimed twice")

	stale, err := e.store.ClaimSource(e.ctx, time.Now().Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, stale, "the reconciler claims a crawl whose lease ran out")

	assert.Error(t, e.store.UpsertPage(e.ctx, *c, knowledge.Page{URL: "https://e.com/x", Title: "x", Sections: []byte(`[]`)}, time.Now()))
	assert.Error(t, e.store.DeletePages(e.ctx, *c, []string{"https://e.com/x"}, time.Now()))
	docs, err := e.svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Empty(t, docs, "the old lease holder writes no page")
	require.NoError(t, e.store.FinishCrawl(e.ctx, *c, nil, time.Now()))
	got, err := e.svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceCrawling, got.Status, "nor ends the new holder's crawl")
}
