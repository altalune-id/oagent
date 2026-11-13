package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge/chunk"
	"altalune.id/oagent/internal/knowledge/extract"
)

const (
	// IngestWorkerName names the ingest worker.
	IngestWorkerName = "knowledge-ingest"
	// MaxIngestPerInstance caps the documents one instance ingests at once.
	MaxIngestPerInstance = 4
	ingestLease          = 15 * time.Minute
	ingestTick           = 5 * time.Second
)

// Extractor turns a file's bytes into sections.
type Extractor interface {
	Extract(ctx context.Context, kind extract.Kind, data []byte) ([]extract.Section, error)
}

// ChildExtractor reads every uploaded format in the limited child process.
type ChildExtractor struct{ Child extract.Child }

// Extract implements Extractor.
func (c ChildExtractor) Extract(ctx context.Context, kind extract.Kind, data []byte) ([]extract.Section, error) {
	return c.Child.Extract(ctx, kind, data)
}

// WithExtractor sets how the ingest worker reads uploaded files.
func (s *Service) WithExtractor(x Extractor) *Service {
	s.extractor = x
	return s
}

// Ingest chunks and embeds one claimed document; a failure a person can act on fails it, any other leaves it to the reconciler.
func (s *Service) Ingest(ctx context.Context, c Claim) error {
	doc := c.Document
	st, err := s.store.Settings(ctx, doc.ProjectID)
	if IsEmbeddingModelNotSetError(err) {
		return s.store.ReleaseDocument(ctx, c, Spend{}, s.now())
	}
	if err != nil {
		return err
	}
	res, err := s.ingest(ctx, doc, st)
	embedded := err == nil
	if embedded {
		err = s.store.ReplaceChunks(ctx, c, *st, res, s.now())
	}
	spent := res.Spend()
	switch {
	case err == nil, errors.Is(err, errLeaseLost):
		return nil
	case errors.Is(err, errModelChanged):
		return s.store.ReleaseDocument(ctx, c, spent, s.now())
	case IsExpectedError(err):
		return s.store.FailDocument(ctx, c, FailureOf(err), spent, s.now())
	case embedded || spent.Tokens > 0:
		// SECURITY: the provider was paid; a retry would pay again for the same text, so the document fails instead.
		crashed := Failure{Code: apperror.CodeKnowledgeCouldNotReadFile, Reason: string(UnreadableCrashed)}
		if ferr := s.store.FailDocument(ctx, c, crashed, spent, s.now()); ferr != nil {
			return errors.Join(err, ferr)
		}
		return fmt.Errorf("knowledge: ingest failed after embedding: %w", err)
	}
	return err
}

func (s *Service) ingest(ctx context.Context, doc *Document, st *Settings) (IngestResult, error) {
	m, err := s.embeddingModel(ctx, st.ProviderID, st.Model)
	if err != nil {
		return IngestResult{}, err
	}
	emb, err := s.embedder(m, st.Model, st.Dims)
	if err != nil {
		return IngestResult{}, err
	}
	chunks, pages, err := s.chunks(ctx, doc)
	if err != nil {
		return IngestResult{}, err
	}
	others, err := s.store.ProjectChunkCount(ctx, doc.ProjectID, doc.ID)
	if err != nil {
		return IngestResult{}, err
	}
	if others+len(chunks) > MaxChunksPerProject {
		return IngestResult{}, &LimitReachedError{Reason: LimitChunksPerProject, Max: MaxChunksPerProject}
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = chunk.EmbedText(doc.Title, c.HeadingPath, c.Text)
	}
	out, err := emb.Embed(ctx, texts)
	paid := IngestResult{Tokens: out.Tokens, CostMicros: out.Tokens * *m.InputPriceMicros / 1_000_000, Pages: pages}
	if err != nil {
		return paid, EmbedError(err)
	}
	if len(out.Vectors) != len(chunks) {
		short := fmt.Errorf("%d vectors for %d chunks", len(out.Vectors), len(chunks))
		return paid, &EmbeddingFailedError{Reason: EmbeddingWrongDims, Cause: short}
	}
	for i := range chunks {
		chunks[i].Embedding = out.Vectors[i]
	}
	paid.Chunks = chunks
	return paid, nil
}

func (s *Service) chunks(ctx context.Context, doc *Document) ([]StoredChunk, int, error) {
	content, err := s.store.DocumentContent(ctx, doc.ID)
	if err != nil {
		return nil, 0, err
	}
	if content == nil {
		stored, err := s.store.ChunkTexts(ctx, doc.ID)
		if err != nil || len(stored) > 0 {
			return stored, doc.PageCount, err
		}
	}
	sections, err := s.sections(ctx, doc, content)
	if err != nil {
		return nil, 0, err
	}
	split, err := chunk.Split(chunk.Doc{Title: doc.Title, URL: doc.URL, Sections: sections})
	if l, ok := chunk.AsLimitError(err); ok {
		reason := LimitChunksPerDocument
		if l.Limit == chunk.LimitTextBytes {
			reason = LimitTextPerDocument
		}
		return nil, 0, &LimitReachedError{Reason: reason, Max: l.Max}
	}
	if err != nil {
		return nil, 0, err
	}
	if len(split) == 0 {
		return nil, 0, &NoTextFoundError{}
	}
	out := make([]StoredChunk, len(split))
	pages := 0
	for i, c := range split {
		out[i] = StoredChunk{Ordinal: c.Ordinal, HeadingPath: c.HeadingPath, Page: c.Page, URL: c.URL, Text: c.Text}
		pages = max(pages, c.Page)
	}
	return out, pages, nil
}

func (s *Service) sections(ctx context.Context, doc *Document, content []byte) ([]extract.Section, error) {
	if content != nil {
		var sections []extract.Section
		if err := json.Unmarshal(content, &sections); err != nil {
			return nil, err
		}
		return sections, nil
	}
	if doc.FileID == nil || !s.StorageConfigured() {
		return nil, &NoTextFoundError{}
	}
	f, err := s.files.File(ctx, *doc.FileID)
	if err != nil {
		return nil, err
	}
	rc, err := s.files.Open(ctx, f.ObjectKey, MaxUploadBytes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if storage.IsObjectTooLargeError(err) {
		return nil, &UploadRejectedError{Reason: RejectSize}
	}
	if err != nil {
		return nil, err
	}
	sections, err := s.extractor.Extract(ctx, kindOf(f.ContentType), data)
	return sections, extractErr(err)
}

func kindOf(contentType string) extract.Kind {
	switch contentType {
	case TypePDF:
		return extract.KindPDF
	case TypeDOCX:
		return extract.KindDOCX
	case TypeMarkdown:
		return extract.KindMarkdown
	}
	return extract.KindText
}

func extractErr(err error) error {
	if err == nil {
		return nil
	}
	if r, ok := extract.AsRejectedError(err); ok {
		return &UploadRejectedError{Reason: RejectReason(r.Reason)}
	}
	if extract.IsNoTextError(err) {
		return &NoTextFoundError{}
	}
	if u, ok := errors.AsType[*extract.UnreadableError](err); ok {
		return &UnreadableError{Reason: UnreadableReason(u.Reason)}
	}
	return err
}

// Tenants runs fn once per org, with ctx scoped to it.
type Tenants interface {
	Each(ctx context.Context, fn func(ctx context.Context, tenantID string) error) error
}

// IngestWorker claims documents in every org and ingests up to MaxIngestPerInstance at once; a stale lease is claimed again.
type IngestWorker struct {
	svc     *Service
	tenants Tenants
	log     *slog.Logger
	kick    chan struct{}
	tick    time.Duration
	pool    *pool
}

// NewIngestWorker builds the knowledge-ingest worker.
func NewIngestWorker(svc *Service, tenants Tenants, log *slog.Logger) *IngestWorker {
	return &IngestWorker{
		svc: svc, tenants: tenants, log: log.With("worker", IngestWorkerName),
		kick: make(chan struct{}, 1), tick: ingestTick, pool: newPool(MaxIngestPerInstance),
	}
}

// Name implements worker.Worker.
func (w *IngestWorker) Name() string { return IngestWorkerName }

// Kick wakes the worker before its next tick; it never blocks.
func (w *IngestWorker) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run implements worker.Worker: one round per tick or kick until ctx is done, then waits for its ingests.
func (w *IngestWorker) Run(ctx context.Context) error {
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

// Round claims as many documents as the instance has free slots, org by org, and ingests each in the background.
func (w *IngestWorker) Round(ctx context.Context) {
	err := w.tenants.Each(ctx, func(orgCtx context.Context, _ string) error {
		free := w.pool.free()
		if free == 0 {
			return nil
		}
		claims, err := w.svc.store.ClaimDocuments(orgCtx, w.svc.now(), ingestLease, free)
		if err != nil {
			return err
		}
		for _, c := range claims {
			w.pool.start(func() { w.ingest(tenant.WithOrg(ctx, c.Document.OrgID), c) })
		}
		return nil
	})
	if err != nil {
		w.log.ErrorContext(ctx, "knowledge.ingest claim", slog.Any("error", err))
	}
}

func (w *IngestWorker) ingest(ctx context.Context, c Claim) {
	renew := func(ctx context.Context) error { return w.svc.store.RenewLease(ctx, c, w.svc.now().Add(ingestLease)) }
	withHeartbeat(ctx, ingestLease/3, renew, func(ctx context.Context) {
		if err := w.svc.Ingest(ctx, c); err != nil && ctx.Err() == nil {
			w.log.ErrorContext(ctx, "knowledge.ingest", slog.String("document", c.Document.PublicID), slog.Any("error", err))
		}
	})
}

// Wait blocks until every ingest the worker started has ended.
func (w *IngestWorker) Wait() { w.pool.wait() }
