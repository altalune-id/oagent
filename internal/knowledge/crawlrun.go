package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge/crawl"
	"altalune.id/oagent/internal/knowledge/extract"
)

const (
	// CrawlWorkerName names the crawl worker.
	CrawlWorkerName = "knowledge-crawl"
	crawlLease      = 10 * time.Minute
	crawlTick       = 10 * time.Second
	// MaxCrawlPerInstance caps the sources one instance crawls at once.
	MaxCrawlPerInstance = 4
	// MaxCrawlPerOrg caps the instance's crawl slots one org holds.
	MaxCrawlPerOrg = 2
)

// Crawler fetches a seed's pages on its host and returns the URLs it visited.
type Crawler interface {
	Crawl(ctx context.Context, seed string, visit func(crawl.Page) error) ([]string, error)
}

// WithCrawler builds crawlers with fn instead of the guarded crawler; tests only.
func (s *Service) WithCrawler(fn func() Crawler) *Service {
	s.newCrawler = fn
	return s
}

func (s *Service) crawler() Crawler {
	if s.newCrawler != nil {
		return s.newCrawler()
	}
	return crawl.New(crawl.Config{AllowPrivateHosts: s.crawlPrivateHosts})
}

// Refresh queues a URL source for a new crawl.
func (s *Service) Refresh(ctx context.Context, sourceID uuid.UUID) (*Source, error) {
	src, err := s.store.SourceByID(ctx, sourceID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: refresh", err)
	}
	if src.Kind != KindURL {
		return nil, &InvalidSourceError{Reason: SourceKindMismatch}
	}
	if src, err = s.store.MarkCrawling(ctx, sourceID, s.now()); err != nil {
		return nil, s.errOrReport(ctx, "knowledge: refresh", err)
	}
	s.wakeCrawl()
	return src, nil
}

// Crawl fetches a claimed source's pages, stores each as a pending document, drops pages that vanished and ends the lease.
func (s *Service) Crawl(ctx context.Context, c SourceClaim) error {
	prev, err := s.store.PageURLs(ctx, c.Source.ID)
	if err != nil {
		return err
	}
	pages, visited := 0, []string(nil)
	if s.extractor == nil {
		err = errNoExtractor
	} else {
		visited, err = s.crawler().Crawl(ctx, c.Source.SeedURL, func(p crawl.Page) error {
			page, ok := pageOf(ctx, s.extractor, p)
			if !ok {
				return nil
			}
			pages++
			return s.store.UpsertPage(ctx, c, page, s.now())
		})
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, errLeaseLost) {
		return nil
	}
	var failure *Failure
	switch {
	case errors.Is(err, errNoExtractor):
		f := FailureOf(err)
		failure = &f
	case err != nil:
		f := FailureOf(crawlErr(err))
		failure = &f
	case pages == 0:
		f := FailureOf(&CrawlFailedError{Reason: CrawlNoPages})
		failure = &f
	default:
		if err := s.store.DeletePages(ctx, c, crawl.Vanished(prev, visited), s.now()); err != nil {
			if errors.Is(err, errLeaseLost) {
				return nil
			}
			return err
		}
	}
	if err := s.store.FinishCrawl(ctx, c, failure, s.now()); err != nil {
		return err
	}
	s.wake()
	return nil
}

var errNoExtractor = errors.New("knowledge: no extractor set; crawled pages cannot be read")

func crawlErr(err error) error {
	if _, ok := crawl.AsOffHostRedirectError(err); ok {
		return &CrawlFailedError{Reason: CrawlOffHostRedirect}
	}
	if cf, ok := errors.AsType[*CrawlFailedError](err); ok {
		return cf
	}
	return &CrawlFailedError{Reason: CrawlUnreachable}
}

func pageOf(ctx context.Context, x Extractor, p crawl.Page) (Page, bool) {
	kind := extract.KindHTML
	switch ct := strings.ToLower(p.ContentType); {
	case strings.Contains(ct, "html"):
	case strings.HasPrefix(ct, "text/markdown"):
		kind = extract.KindMarkdown
	case strings.HasPrefix(ct, "text/plain"):
		kind = extract.KindText
	default:
		return Page{}, false
	}
	sections, err := x.Extract(ctx, kind, p.Body)
	if err != nil || len(sections) == 0 {
		return Page{}, false
	}
	raw, err := json.Marshal(sections)
	if err != nil || len(raw) > MaxPageContentBytes {
		return Page{}, false
	}
	title := p.URL
	for _, sec := range sections {
		if sec.Heading != "" {
			title = sec.Heading
			break
		}
	}
	if len([]rune(title)) > MaxTitleLen {
		title = string([]rune(title)[:MaxTitleLen])
	}
	sum := sha256.Sum256(raw)
	return Page{URL: p.URL, Title: title, Sections: raw, Hash: hex.EncodeToString(sum[:])}, true
}

// WithCrawlKick lets Refresh and CreateSource wake the crawl worker.
func (s *Service) WithCrawlKick(kick func()) *Service {
	s.crawlKick = kick
	return s
}

func (s *Service) wakeCrawl() {
	if s.crawlKick != nil {
		s.crawlKick()
	}
}

// CrawlWorker crawls up to MaxCrawlPerInstance URL sources at once, claiming one per org per round.
type CrawlWorker struct {
	svc     *Service
	tenants Tenants
	log     *slog.Logger
	kick    chan struct{}
	tick    time.Duration
	pool    *pool

	mu   sync.Mutex
	busy map[uuid.UUID]int
}

// NewCrawlWorker builds the knowledge-crawl worker.
func NewCrawlWorker(svc *Service, tenants Tenants, log *slog.Logger) *CrawlWorker {
	return &CrawlWorker{
		svc: svc, tenants: tenants, log: log.With("worker", CrawlWorkerName),
		kick: make(chan struct{}, 1), tick: crawlTick, pool: newPool(MaxCrawlPerInstance), busy: map[uuid.UUID]int{},
	}
}

// Name implements worker.Worker.
func (w *CrawlWorker) Name() string { return CrawlWorkerName }

func (w *CrawlWorker) take(org uuid.UUID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.busy[org] >= MaxCrawlPerOrg {
		return false
	}
	w.busy[org]++
	return true
}

func (w *CrawlWorker) give(org uuid.UUID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.busy[org]--
	if w.busy[org] <= 0 {
		delete(w.busy, org)
	}
}

// Kick wakes the worker before its next tick; it never blocks.
func (w *CrawlWorker) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run implements worker.Worker: one round per tick or kick until ctx is done, then waits for its crawls.
func (w *CrawlWorker) Run(ctx context.Context) error {
	defer w.Wait()
	t := time.NewTicker(w.tick)
	defer t.Stop()
	for {
		w.Round(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		case <-w.kick:
		}
	}
}

// Round claims one crawlable source per org while slots are free, at most MaxCrawlPerOrg per org, and crawls each in the background.
func (w *CrawlWorker) Round(ctx context.Context) {
	err := w.tenants.Each(ctx, func(orgCtx context.Context, _ string) error {
		tc, err := tenant.From(orgCtx)
		if err != nil {
			return err
		}
		if w.pool.free() == 0 || !w.take(tc.OrgID) {
			return nil
		}
		c, err := w.svc.store.ClaimSource(orgCtx, w.svc.now(), crawlLease)
		if err != nil || c == nil {
			w.give(tc.OrgID)
			return err
		}
		w.pool.start(func() {
			defer w.give(tc.OrgID)
			w.crawl(tenant.WithOrg(ctx, c.Source.OrgID), *c)
		})
		return nil
	})
	if err != nil {
		w.log.ErrorContext(ctx, "knowledge.crawl claim", slog.Any("error", err))
	}
}

func (w *CrawlWorker) crawl(ctx context.Context, c SourceClaim) {
	renew := func(ctx context.Context) error {
		return w.svc.store.RenewSourceLease(ctx, c, w.svc.now().Add(crawlLease))
	}
	withHeartbeat(ctx, crawlLease/3, renew, func(ctx context.Context) {
		if err := w.svc.Crawl(ctx, c); err != nil && ctx.Err() == nil {
			w.log.ErrorContext(ctx, "knowledge.crawl", slog.String("source", c.Source.PublicID), slog.Any("error", err))
		}
	})
}

// Wait blocks until every crawl the worker started has ended.
func (w *CrawlWorker) Wait() { w.pool.wait() }
