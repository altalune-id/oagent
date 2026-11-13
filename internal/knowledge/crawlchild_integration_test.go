//go:build integration

package knowledge_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/extract"
)

// SECURITY: a crawled page is parsed in the limited child process too; with a broken child no page becomes a document.
func TestCrawl_PagesAreExtractedInTheChild(t *testing.T) {
	e := newEnv(t)
	svc := e.crawlWith(fakeCrawler{pages: []crawl.Page{html("https://help.example.com/", "<h1>Help</h1><p>Start here.</p>")}}).
		WithExtractor(knowledge.ChildExtractor{Child: extract.Child{Path: "/nonexistent/oagent"}})
	src := e.urlSource(t, svc)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	docs, err := svc.ListDocuments(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestCrawl_WithoutAnExtractorFailsTheSource(t *testing.T) {
	e := newEnv(t)
	svc := e.svc.WithCrawler(func() knowledge.Crawler {
		return fakeCrawler{pages: []crawl.Page{html("https://help.example.com/", "<h1>Help</h1><p>Start here.</p>")}}
	})
	src := e.urlSource(t, svc)
	runRound(t, knowledge.NewCrawlWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetSource(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.SourceFailed, got.Status)
	assert.Equal(t, &knowledge.Failure{Code: apperror.CodeUnexpected}, got.Failure)
}
