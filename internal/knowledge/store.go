package knowledge

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SourceRef names a source and the org and project it lives in.
type SourceRef struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}

// DocumentRef names a document and where it lives.
type DocumentRef struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	SourceID  uuid.UUID
}

// DocumentCounts sums a source's documents by state.
type DocumentCounts struct {
	Total  int
	Ready  int
	Failed int
}

// Claim is a document leased to one ingest run; Token proves the lease.
type Claim struct {
	Document *Document
	Token    string
}

// StoredChunk is one chunk of a document, with its vector when written.
type StoredChunk struct {
	Ordinal     int
	HeadingPath string
	Page        int
	URL         string
	Text        string
	Embedding   []float32
}

// IngestResult is what one ingest run adds to its document.
type IngestResult struct {
	Chunks     []StoredChunk
	Tokens     int64
	CostMicros int64
	Pages      int
}

// Spend is what one ingest attempt paid the embeddings provider; it is added to the document's totals.
type Spend struct {
	Tokens     int64
	CostMicros int64
}

// Spend is the result's provider spend.
func (r IngestResult) Spend() Spend { return Spend{Tokens: r.Tokens, CostMicros: r.CostMicros} }

// Store persists sources and documents.
type Store interface {
	Reader
	Writer
}

// Reader reads sources and documents of the org on ctx.
type Reader interface {
	ViewReader
	SourceByID(ctx context.Context, id uuid.UUID) (*Source, error)
	ListSources(ctx context.Context, projectID uuid.UUID) ([]*Source, error)
	DocumentCounts(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID]DocumentCounts, error)
	ResolveSource(ctx context.Context, publicID string) (SourceRef, error)
	DocumentByID(ctx context.Context, id uuid.UUID) (*Document, error)
	ListDocuments(ctx context.Context, sourceID uuid.UUID) ([]*Document, error)
	ResolveDocument(ctx context.Context, publicID string) (DocumentRef, error)
	Settings(ctx context.Context, projectID uuid.UUID) (*Settings, error)
	EmbeddingModelOf(ctx context.Context, providerID uuid.UUID) (string, error)
	ChunkTexts(ctx context.Context, documentID uuid.UUID) ([]StoredChunk, error)
	DocumentContent(ctx context.Context, documentID uuid.UUID) ([]byte, error)
	PageURLs(ctx context.Context, sourceID uuid.UUID) ([]string, error)
}

// Writer writes sources and documents; the lock methods need a unit of work on ctx and hold their lock until it ends.
type Writer interface {
	InsertSource(ctx context.Context, s *Source) error
	LockSource(ctx context.Context, id uuid.UUID) (*Source, error)
	CountLiveDocuments(ctx context.Context, sourceID uuid.UUID) (int, error)
	InsertDocument(ctx context.Context, d *Document) error
	LockDocument(ctx context.Context, id uuid.UUID) (*Document, error)
	LockDocumentByFile(ctx context.Context, fileID uuid.UUID) (*Document, error)
	SaveDocument(ctx context.Context, d *Document) error
	LockSettings(ctx context.Context, projectID uuid.UUID) (*Settings, error)
	SaveSettings(ctx context.Context, s *Settings) error
	RequeueDocuments(ctx context.Context, projectID uuid.UUID) error
	ClaimDocuments(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Claim, error)
	ReleaseDocument(ctx context.Context, c Claim, spent Spend, now time.Time) error
	FailDocument(ctx context.Context, c Claim, f Failure, spent Spend, now time.Time) error
	ProjectChunkCount(ctx context.Context, projectID, exceptDocument uuid.UUID) (int, error)
	ReplaceChunks(ctx context.Context, c Claim, st Settings, res IngestResult, now time.Time) error
	MarkCrawling(ctx context.Context, sourceID uuid.UUID, now time.Time) (*Source, error)
	ClaimSource(ctx context.Context, now time.Time, lease time.Duration) (*SourceClaim, error)
	UpsertPage(ctx context.Context, c SourceClaim, page Page, now time.Time) error
	DeletePages(ctx context.Context, c SourceClaim, urls []string, now time.Time) error
	RenewLease(ctx context.Context, c Claim, until time.Time) error
	RenewSourceLease(ctx context.Context, c SourceClaim, until time.Time) error
	FinishCrawl(ctx context.Context, c SourceClaim, f *Failure, now time.Time) error
	LockSourcesShared(ctx context.Context, ids []uuid.UUID) error
	DeleteSource(ctx context.Context, id uuid.UUID) error
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	SourceFileIDs(ctx context.Context, sourceID uuid.UUID) ([]uuid.UUID, error)
}

// SourceClaim is a URL source leased to one crawl; Token proves the lease.
type SourceClaim struct {
	Source *Source
	Token  string
}

// Page is one crawled page: its title, its extracted sections as JSON and their SHA-256 in hex.
type Page struct {
	URL      string
	Title    string
	Sections []byte
	Hash     string
}
