package knowledge

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/weburl"
)

// Kind is where a source's documents come from.
type Kind string

// The source kinds.
const (
	KindUpload Kind = "upload"
	KindURL    Kind = "url"
)

// Backend is the store that indexes and searches a source's documents.
type Backend string

// BackendPgvector is the built-in Postgres store.
const BackendPgvector Backend = "pgvector"

// SourceStatus is where a source's crawl is.
type SourceStatus string

// The source statuses; an upload source stays idle.
const (
	SourceIdle     SourceStatus = "idle"
	SourceCrawling SourceStatus = "crawling"
	SourceFailed   SourceStatus = "failed"
)

// DocumentStatus is where a document is in ingestion.
type DocumentStatus string

// The document statuses.
const (
	DocumentUploading  DocumentStatus = "uploading"
	DocumentPending    DocumentStatus = "pending"
	DocumentProcessing DocumentStatus = "processing"
	DocumentReady      DocumentStatus = "ready"
	DocumentFailed     DocumentStatus = "failed"
)

const (
	publicIDLen = 21
	// MaxNameLen bounds a source name, in characters.
	MaxNameLen = 200
	// MaxTitleLen bounds a document title, in characters.
	MaxTitleLen = 255
)

// Upload content types.
const (
	TypePDF      = "application/pdf"
	TypeDOCX     = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	TypeMarkdown = "text/markdown"
	TypeText     = "text/plain"
)

// SupportedDims lists the embedding sizes the store indexes.
//
//nolint:gochecknoglobals // read-only list.
var SupportedDims = []int{768, 1024, 1536, 3072}

// Settings is a project's embedding model: every chunk of the project is embedded with it.
type Settings struct {
	OrgID      uuid.UUID
	ProjectID  uuid.UUID
	ProviderID uuid.UUID
	Model      string
	Dims       int
	UpdatedAt  time.Time
}

// ModelKey names the model and size that wrote a chunk; search reads only chunks of the current key.
func (s Settings) ModelKey() string {
	return s.ProviderID.String() + "/" + strconv.Itoa(s.Dims) + "/" + s.Model
}

// Failure is the code, and its reason if any, that failed a source or a document.
type Failure struct {
	Code   apperror.Code
	Reason string
}

// FailureOf names err's code and reason; an error with no code is the unexpected one.
func FailureOf(err error) Failure {
	ae, ok := apperror.AsAppError(err)
	if !ok {
		return Failure{Code: apperror.CodeUnexpected}
	}
	return Failure{Code: ae.Code(), Reason: ae.Meta()["reason"]}
}

// Source is a named set of documents in a project.
type Source struct {
	ID        uuid.UUID
	PublicID  string
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	Kind      Kind
	Backend   Backend
	Name      string
	SeedURL   string
	Status    SourceStatus
	Failure   *Failure
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SourceInput is what a person declares for a new source.
type SourceInput struct {
	Kind    Kind
	Backend Backend
	Name    string
	SeedURL string
}

// NewSource validates in against the enabled backends; a URL source starts crawling.
func NewSource(orgID, projectID uuid.UUID, in SourceInput, enabled []Backend, now time.Time) (*Source, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return nil, apperror.Invalid("name", apperror.FieldRequired)
	case utf8.RuneCountInString(name) > MaxNameLen:
		return nil, apperror.Invalid("name", apperror.FieldTooLong)
	}
	if !slices.Contains(enabled, in.Backend) {
		return nil, &InvalidSourceError{Reason: SourceBackendDisabled}
	}
	s := &Source{OrgID: orgID, ProjectID: projectID, Kind: in.Kind, Backend: in.Backend, Name: name, Status: SourceIdle}
	switch in.Kind {
	case KindURL:
		seed, reason := weburl.Check(in.SeedURL, false)
		if reason != "" {
			return nil, &InvalidSourceError{Reason: SourceReason(reason)}
		}
		s.SeedURL, s.Status = seed, SourceCrawling
	case KindUpload:
	default:
		return nil, apperror.Invalid("source", apperror.FieldInvalid)
	}
	if err := s.mint(now); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Source) mint(now time.Time) error {
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return err
	}
	now = now.UTC()
	s.ID, s.PublicID, s.CreatedAt, s.UpdatedAt = uuid.New(), publicID, now, now
	return nil
}

// Document is one file or page of a source.
type Document struct {
	ID              uuid.UUID
	PublicID        string
	OrgID           uuid.UUID
	ProjectID       uuid.UUID
	SourceID        uuid.UUID
	Title           string
	FileID          *uuid.UUID
	URL             string
	Status          DocumentStatus
	Failure         *Failure
	PageCount       int
	EmbedTokens     int64
	EmbedCostMicros int64
	Attempts        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UploadInput is what the browser declares before it sends a file.
type UploadInput struct {
	Filename    string
	ContentType string
	Size        int64
}

// NewUploadDocument starts a document of an upload source, waiting for fileID's bytes.
func NewUploadDocument(src *Source, title string, fileID uuid.UUID, now time.Time) (*Document, error) {
	if src.Kind != KindUpload {
		return nil, &InvalidSourceError{Reason: SourceKindMismatch}
	}
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return nil, apperror.Invalid("filename", apperror.FieldRequired)
	case utf8.RuneCountInString(title) > MaxTitleLen:
		return nil, apperror.Invalid("filename", apperror.FieldTooLong)
	case HasControlCharacter(title):
		return nil, apperror.Invalid("filename", apperror.FieldInvalid)
	}
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	return &Document{
		ID: uuid.New(), PublicID: publicID, OrgID: src.OrgID, ProjectID: src.ProjectID, SourceID: src.ID,
		Title: title, FileID: &fileID, Status: DocumentUploading, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Fail marks the document failed with err's code and reason.
func (d *Document) Fail(err error, now time.Time) {
	f := FailureOf(err)
	d.Status, d.Failure, d.UpdatedAt = DocumentFailed, &f, now.UTC()
}
