package knowledge

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/extract"
)

func TestKindOf(t *testing.T) {
	assert.Equal(t, extract.KindPDF, kindOf(TypePDF))
	assert.Equal(t, extract.KindDOCX, kindOf(TypeDOCX))
	assert.Equal(t, extract.KindMarkdown, kindOf(TypeMarkdown))
	assert.Equal(t, extract.KindText, kindOf(TypeText))
}

func TestExtractErr_MapsToCodes(t *testing.T) {
	assert.NoError(t, extractErr(nil))
	var rej *UploadRejectedError
	assert.ErrorAs(t, extractErr(&extract.RejectedError{Reason: extract.TooManyPages}), &rej)
	assert.Equal(t, RejectTooManyPages, rej.Reason)
	assert.True(t, IsNoTextFoundError(extractErr(&extract.NoTextError{})))
	var un *UnreadableError
	assert.ErrorAs(t, extractErr(&extract.UnreadableError{Reason: extract.Timeout}), &un)
	assert.Equal(t, UnreadableTimeout, un.Reason)
	boom := errors.New("boom")
	assert.Equal(t, boom, extractErr(boom))
}

func TestCrawlErr_ExplainsTheFailure(t *testing.T) {
	for err, want := range map[error]CrawlReason{
		&crawl.OffHostRedirectError{Host: "www.e.com"}:   CrawlOffHostRedirect,
		&CrawlFailedError{Reason: CrawlRobotsDisallowed}: CrawlRobotsDisallowed,
		errors.New("dial"): CrawlUnreachable,
	} {
		cf, ok := errors.AsType[*CrawlFailedError](crawlErr(err))
		assert.True(t, ok)
		assert.Equal(t, want, cf.Reason)
	}
}

func TestPageOf_TextFormatsOnly(t *testing.T) {
	ctx := context.Background()
	md, ok := pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/a.md", ContentType: "text/markdown", Body: []byte("# Title\n\nBody")})
	assert.True(t, ok)
	assert.Equal(t, "Title", md.Title)
	txt, ok := pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/a.txt", ContentType: "text/plain", Body: []byte("just text")})
	assert.True(t, ok)
	assert.Equal(t, "https://e.com/a.txt", txt.Title, "no heading: the URL names the page")
	_, ok = pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/a.png", ContentType: "image/png", Body: []byte{1}})
	assert.False(t, ok)
	_, ok = pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/", ContentType: "text/html", Body: []byte("<nav>only chrome</nav>")})
	assert.False(t, ok, "no text, no document")
	long, ok := pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/l", ContentType: "text/html", Body: []byte("<h1>" + strings.Repeat("t", 300) + "</h1><p>x</p>")})
	assert.True(t, ok)
	assert.Len(t, []rune(long.Title), MaxTitleLen)
	_, ok = pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/big", ContentType: "text/plain", Body: []byte(strings.Repeat("word ", MaxPageContentBytes/5+1))})
	assert.False(t, ok, "a page over the content cap is not stored")
	small, ok := pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/s", ContentType: "text/plain", Body: []byte("same")})
	require.True(t, ok)
	again, _ := pageOf(ctx, ChildExtractorForTests(), crawl.Page{URL: "https://e.com/s", ContentType: "text/plain", Body: []byte("same")})
	assert.Len(t, small.Hash, 64)
	assert.Equal(t, small.Hash, again.Hash)
}

func TestBrowserUpload_DropsForbiddenHeaders(t *testing.T) {
	up := browserUpload(storage.Upload{Headers: map[string]string{"content-length": "1", "Host": "h", "Content-Type": "text/plain", "Connection": "x"}})
	assert.Equal(t, map[string]string{"Content-Type": "text/plain"}, up.Headers)
}

type runner interface {
	Run(ctx context.Context) error
	Kick()
	Name() string
}

type countingTenants struct{ n atomic.Int32 }

func (c *countingTenants) Each(context.Context, func(context.Context, string) error) error {
	c.n.Add(1)
	return errors.New("tenants down")
}

func TestWorkers_TickKickAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for name, mk := range map[string]func(*countingTenants) runner{
			"ingest": func(c *countingTenants) runner {
				return NewIngestWorker(&Service{now: time.Now}, c, slog.New(slog.DiscardHandler))
			},
			"crawl": func(c *countingTenants) runner {
				return NewCrawlWorker(&Service{now: time.Now}, c, slog.New(slog.DiscardHandler))
			},
		} {
			tenants := &countingTenants{}
			w := mk(tenants)
			assert.NotEmpty(t, w.Name(), name)
			idle := mk(&countingTenants{})
			idle.Kick()
			idle.Kick()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- w.Run(ctx) }()
			synctest.Wait()
			assert.Equal(t, int32(1), tenants.n.Load(), "%s: a round at start", name)
			w.Kick()
			synctest.Wait()
			assert.Equal(t, int32(2), tenants.n.Load(), "%s: a kick runs a round", name)
			time.Sleep(time.Minute)
			synctest.Wait()
			assert.Greater(t, tenants.n.Load(), int32(2), "%s: the ticker runs rounds, a failed one included", name)
			cancel()
			assert.NoError(t, <-done, name)
		}
	})
}

func TestProtoMappers_EveryValue(t *testing.T) {
	for _, s := range []DocumentStatus{DocumentUploading, DocumentPending, DocumentProcessing, DocumentReady, DocumentFailed} {
		assert.NotZero(t, documentStatusToProto(s), s)
	}
	assert.Zero(t, documentStatusToProto("gone"))
	for _, s := range []SourceStatus{SourceIdle, SourceCrawling, SourceFailed} {
		assert.NotZero(t, sourceStatusToProto(s), s)
	}
	assert.Zero(t, sourceStatusToProto("gone"))
	assert.Zero(t, kindToProto("gone"))
	assert.Zero(t, backendToProto("gone"))
	assert.Nil(t, failureToProto(nil))
	assert.Equal(t, int32(1<<30), count(1<<40))
}
