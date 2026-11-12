package knowledge

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge/embed"
)

// Service is the knowledge application service: the driving port surfaces and workers call.
type Service struct {
	store      Store
	uow        db.Transactor
	files      Files
	projects   Projects
	backends   []Backend
	unexpected apperror.UnexpectedFunc
	orgs       tenant.ProjectResolver
	now        func() time.Time

	models            Models
	bindings          Bindings
	retrievers        map[Backend]Retriever
	extractor         Extractor
	kick              func()
	crawlKick         func()
	newCrawler        func() Crawler
	newEmbedder       func(embed.Config) (Embedder, error)
	allowPrivateHosts bool
	crawlPrivateHosts bool
}

// NewService builds a Service over store; uploads need files with a driver, and sources may only use the enabled backends.
func NewService(
	store Store, uow db.Transactor, files Files, projects Projects, backends []Backend, unexpected apperror.UnexpectedFunc,
) *Service {
	return &Service{
		store: store, uow: uow, files: files, projects: projects, backends: backends, unexpected: unexpected, now: time.Now,
		newEmbedder: newGuardedEmbedder,
	}
}

// WithProjects makes CreateSource refuse a project outside the org on ctx; nil skips the check.
func (s *Service) WithProjects(r tenant.ProjectResolver) *Service {
	s.orgs = r
	return s
}

// Backends lists the backends a new source may use.
func (s *Service) Backends() []Backend { return slices.Clone(s.backends) }

// StorageConfigured reports whether upload sources can take files.
func (s *Service) StorageConfigured() bool { return s.files != nil && s.files.Enabled() }

// CreateSource validates and stores a source in a project of the org on ctx.
func (s *Service) CreateSource(ctx context.Context, projectID uuid.UUID, in SourceInput) (*Source, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if err := tenant.RequireProjectInOrg(ctx, s.orgs, tc.OrgID, projectID); err != nil {
		if tenant.IsProjectOutsideOrgError(err) {
			return nil, err
		}
		return nil, s.errOrReport(ctx, "knowledge: project org", err)
	}
	src, err := NewSource(tc.OrgID, projectID, in, s.backends, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.store.InsertSource(ctx, src); err != nil {
		return nil, s.errOrReport(ctx, "knowledge: create source", err)
	}
	if src.Status == SourceCrawling {
		s.wakeCrawl()
	}
	return src, nil
}

// GetSource loads a source by id.
func (s *Service) GetSource(ctx context.Context, id uuid.UUID) (*Source, error) {
	src, err := s.store.SourceByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: get source", err)
	}
	return src, nil
}

// SourceView is a source with its document counts and the agents whose published version searches it.
type SourceView struct {
	Source    *Source
	Documents DocumentCounts
	UsedBy    []UsedBy
}

// ListSources returns a project's sources with their document counts.
func (s *Service) ListSources(ctx context.Context, projectID uuid.UUID) ([]SourceView, error) {
	srcs, err := s.store.ListSources(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: list sources", err)
	}
	counts, err := s.store.DocumentCounts(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: document counts", err)
	}
	used, err := s.usedBy(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: used by", err)
	}
	out := make([]SourceView, 0, len(srcs))
	for _, src := range srcs {
		out = append(out, SourceView{Source: src, Documents: counts[src.ID], UsedBy: used[src.ID]})
	}
	return out, nil
}

// ResolveSource names publicID's source and its org, in any org; the caller checks reach before it reads the source.
func (s *Service) ResolveSource(ctx context.Context, publicID string) (SourceRef, error) {
	r, err := s.store.ResolveSource(ctx, publicID)
	if err != nil {
		return SourceRef{}, s.errOrReport(ctx, "knowledge: resolve source", err)
	}
	return r, nil
}

// ResolveDocument names publicID's document and where it lives, in any org; the caller checks reach first.
func (s *Service) ResolveDocument(ctx context.Context, publicID string) (DocumentRef, error) {
	r, err := s.store.ResolveDocument(ctx, publicID)
	if err != nil {
		return DocumentRef{}, s.errOrReport(ctx, "knowledge: resolve document", err)
	}
	return r, nil
}

// GetDocument loads a document by id.
func (s *Service) GetDocument(ctx context.Context, id uuid.UUID) (*Document, error) {
	d, err := s.store.DocumentByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: get document", err)
	}
	return d, nil
}

// ListDocuments returns a source's documents.
func (s *Service) ListDocuments(ctx context.Context, sourceID uuid.UUID) ([]*Document, error) {
	docs, err := s.store.ListDocuments(ctx, sourceID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: list documents", err)
	}
	return docs, nil
}

// BeginUpload records an uploading document and a pending file in one unit of work and signs the browser's PUT.
func (s *Service) BeginUpload(ctx context.Context, sourceID uuid.UUID, in UploadInput) (*Document, storage.Upload, error) {
	if !s.StorageConfigured() {
		return nil, storage.Upload{}, &StorageNotConfiguredError{}
	}
	var (
		doc *Document
		up  storage.Upload
	)
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		src, err := s.store.LockSource(ctx, sourceID)
		if err != nil {
			return err
		}
		if src.Kind != KindUpload {
			return &InvalidSourceError{Reason: SourceKindMismatch}
		}
		n, err := s.store.CountLiveDocuments(ctx, src.ID)
		if err != nil {
			return err
		}
		if n >= MaxDocumentsPerSource {
			return &LimitReachedError{Reason: LimitDocumentsPerSource, Max: MaxDocumentsPerSource}
		}
		projectPublicID, err := s.projects.ProjectPublicID(ctx, src.ProjectID)
		if err != nil {
			return err
		}
		f, signed, err := s.files.BeginUpload(ctx, storage.BeginUploadInput{
			Purpose: UploadPurpose, Owner: storage.Owner{OrgID: src.OrgID, ID: src.ProjectID, PublicID: projectPublicID},
			ContentType: in.ContentType, Size: in.Size,
		})
		if err != nil {
			return uploadErr(err)
		}
		if doc, err = NewUploadDocument(src, in.Filename, f.ID, s.now()); err != nil {
			return err
		}
		up = browserUpload(signed)
		return s.store.InsertDocument(ctx, doc)
	})
	if err != nil {
		return nil, storage.Upload{}, s.errOrReport(ctx, "knowledge: begin upload", err)
	}
	return doc, up, nil
}

//nolint:gochecknoglobals // read-only list.
var forbiddenHeaders = []string{"Content-Length", "Host", "Connection"}

// SECURITY: a browser refuses to set forbidden headers such as Content-Length; the signature still binds the length.
func browserUpload(up storage.Upload) storage.Upload {
	headers := make(map[string]string, len(up.Headers))
	for k, v := range up.Headers {
		if !slices.ContainsFunc(forbiddenHeaders, func(f string) bool { return strings.EqualFold(f, k) }) {
			headers[k] = v
		}
	}
	up.Headers = headers
	return up
}

// CompleteUpload verifies a document's file and, in one unit of work, attaches it and queues the document; a retry is a no-op.
func (s *Service) CompleteUpload(ctx context.Context, documentID uuid.UUID) (*Document, error) {
	if !s.StorageConfigured() {
		return nil, &StorageNotConfiguredError{}
	}
	doc, err := s.store.DocumentByID(ctx, documentID)
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: complete upload", err)
	}
	if doc.Status != DocumentUploading || doc.FileID == nil {
		return doc, nil
	}
	_, verr := s.files.Verify(ctx, *doc.FileID, UploadPurpose, storage.Owner{OrgID: doc.OrgID, ID: doc.ProjectID})
	if verr != nil {
		return nil, s.verifyFailed(ctx, doc.ID, verr)
	}
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if doc, err = s.store.LockDocument(ctx, documentID); err != nil {
			return err
		}
		if doc.Status != DocumentUploading || doc.FileID == nil {
			return nil
		}
		if err := s.files.MarkReady(ctx, *doc.FileID); err != nil {
			return err
		}
		doc.Status, doc.UpdatedAt = DocumentPending, s.now().UTC()
		return s.store.SaveDocument(ctx, doc)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "knowledge: complete upload", err)
	}
	s.wake()
	return doc, nil
}

// WithKick lets the service wake the ingest worker when a document is queued.
func (s *Service) WithKick(kick func()) *Service {
	s.kick = kick
	return s
}

func (s *Service) wake() {
	if s.kick != nil {
		s.kick()
	}
}

func (s *Service) verifyFailed(ctx context.Context, documentID uuid.UUID, verr error) error {
	rejected, ok := rejection(verr)
	if !ok {
		return s.errOrReport(ctx, "knowledge: verify upload", verr)
	}
	if rejected.Reason == RejectMissing {
		return rejected
	}
	lock := func(ctx context.Context) (*Document, error) { return s.store.LockDocument(ctx, documentID) }
	if err := s.failUploading(ctx, lock, rejected); err != nil {
		return s.errOrReport(ctx, "knowledge: fail upload", err)
	}
	return rejected
}

func rejection(err error) (*UploadRejectedError, bool) {
	if storage.IsUploadNotFoundError(err) {
		return &UploadRejectedError{Reason: RejectExpired}, true
	}
	if mm, ok := errors.AsType[*storage.UploadMismatchError](err); ok {
		return &UploadRejectedError{Reason: RejectReason(mm.Reason)}, true
	}
	return nil, false
}

// UploadAbandoned implements storage.Abandoned: a document still waiting for the dropped file fails as expired.
func (s *Service) UploadAbandoned(ctx context.Context, fileID uuid.UUID) error {
	err := s.failUploading(ctx, func(ctx context.Context) (*Document, error) { return s.store.LockDocumentByFile(ctx, fileID) },
		&UploadRejectedError{Reason: RejectExpired})
	if IsDocumentNotFoundError(err) {
		return nil
	}
	return err
}

func (s *Service) failUploading(ctx context.Context, lock func(context.Context) (*Document, error), cause error) error {
	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
		doc, err := lock(ctx)
		if err != nil {
			return err
		}
		if doc.Status != DocumentUploading {
			return nil
		}
		doc.Fail(cause, s.now())
		return s.store.SaveDocument(ctx, doc)
	})
}

func uploadErr(err error) error {
	switch {
	case storage.IsNotConfiguredError(err):
		return &StorageNotConfiguredError{}
	case storage.IsTypeNotAllowedError(err):
		return apperror.Invalid("content_type", apperror.FieldNotAllowed)
	case storage.IsTooLargeError(err):
		return apperror.Invalid("size_bytes", apperror.FieldTooLong)
	}
	return err
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsExpectedError(err) || tenant.IsProjectOutsideOrgError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
