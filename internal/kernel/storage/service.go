package storage

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/nanoid"
)

const publicIDLen = 21

// Service signs uploads, verifies them and hands removed files to the sweep; with no driver every call returns *NotConfiguredError.
type Service struct {
	driver     Driver
	store      Store
	uow        db.Transactor
	cfg        Config
	unexpected apperror.UnexpectedFunc
	now        func() time.Time
}

// NewService builds the feature over driver and store, with uow for the sweep's claims; a nil driver disables it.
func NewService(driver Driver, store Store, uow db.Transactor, cfg Config, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{driver: driver, store: store, uow: uow, cfg: cfg, unexpected: unexpected, now: time.Now}
}

// WithClock replaces the service's clock; tests only.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Enabled reports whether this server has file storage.
func (s *Service) Enabled() bool { return s != nil && s.driver != nil }

// BeginUploadInput is what a domain declares about an upload before the browser sends it.
type BeginUploadInput struct {
	Purpose     Purpose
	Owner       Owner
	ContentType string
	Size        int64
}

// BeginUpload checks the declaration against the purpose, records a pending file and signs its upload; it joins the caller's unit of work.
func (s *Service) BeginUpload(ctx context.Context, in BeginUploadInput) (*File, Upload, error) {
	if !s.Enabled() {
		return nil, Upload{}, &NotConfiguredError{}
	}
	p := in.Purpose
	if !p.Allows(in.ContentType) {
		return nil, Upload{}, &TypeNotAllowedError{Purpose: p.Name, ContentType: in.ContentType}
	}
	if in.Size < 1 {
		return nil, Upload{}, apperror.Invalid("size_bytes", apperror.FieldRequired)
	}
	if in.Size > p.MaxBytes {
		return nil, Upload{}, &TooLargeError{Purpose: p.Name, MaxBytes: p.MaxBytes}
	}
	ctx = tenant.WithOrg(ctx, in.Owner.OrgID)
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return nil, Upload{}, s.unexpected(ctx, "storage: begin upload", err)
	}
	now := s.now().UTC()
	f := &File{
		ID: uuid.New(), PublicID: publicID, OrgID: in.Owner.OrgID, ProjectID: in.Owner.ID,
		Purpose: p.Name, Visibility: p.Visibility,
		ObjectKey:   Key(s.cfg.App, p, in.Owner.PublicID, publicID, Extension(in.ContentType)),
		ContentType: in.ContentType, SizeBytes: in.Size,
		Status: StatusPending, CreatedAt: now, ExpiresAt: now.Add(s.cfg.PendingTTL),
	}
	if err := s.store.Insert(ctx, f); err != nil {
		return nil, Upload{}, s.unexpected(ctx, "storage: begin upload", err)
	}
	up, err := s.driver.PresignPut(ctx, f.ObjectKey, f.ContentType, f.SizeBytes, s.cfg.UploadURLTTL)
	if err != nil {
		return nil, Upload{}, s.unexpected(ctx, "storage: begin upload", err)
	}
	return f, up, nil
}

// Verify checks a finished upload against its declaration, outside any transaction; a mismatch deletes the object.
func (s *Service) Verify(ctx context.Context, fileID uuid.UUID, p Purpose, owner Owner) (*File, error) {
	if !s.Enabled() {
		return nil, &NotConfiguredError{}
	}
	ctx = tenant.WithOrg(ctx, owner.OrgID)
	f, err := s.store.ByID(ctx, fileID)
	if err != nil {
		return nil, s.errOrReport(ctx, "storage: verify", err)
	}
	// SECURITY: an upload made for another project or purpose answers the same as an unknown one.
	if f.ProjectID != owner.ID || f.Purpose != p.Name {
		return nil, &UploadNotFoundError{PublicID: f.PublicID}
	}
	switch {
	case f.Status == StatusReady:
		return f, nil
	case f.Status != StatusPending, !s.now().Before(f.ExpiresAt):
		return nil, &UploadNotFoundError{PublicID: f.PublicID}
	}
	obj, err := s.driver.Head(ctx, f.ObjectKey)
	if IsObjectNotFoundError(err) {
		return nil, &UploadMismatchError{PublicID: f.PublicID, Reason: MismatchMissing}
	}
	if err != nil {
		return nil, s.unexpected(ctx, "storage: verify", err)
	}
	reason := mismatch(f, obj)
	if reason == "" {
		head, err := s.driver.ReadPrefix(ctx, f.ObjectKey, SniffLen)
		if err != nil {
			return nil, s.unexpected(ctx, "storage: verify", err)
		}
		if !Sniff(f.ContentType, head) {
			reason = MismatchContent
		}
	}
	if reason != "" {
		return nil, s.reject(ctx, f, reason)
	}
	return f, nil
}

func mismatch(f *File, obj Object) MismatchReason {
	switch {
	case obj.Size > f.SizeBytes || obj.Size < 1:
		return MismatchSize
	case !strings.EqualFold(mediaType(obj.ContentType), f.ContentType):
		return MismatchType
	}
	return ""
}

func mediaType(contentType string) string {
	mt, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(mt)
}

func (s *Service) reject(ctx context.Context, f *File, reason MismatchReason) error {
	if err := s.driver.Delete(ctx, f.ObjectKey); err != nil {
		return s.unexpected(ctx, "storage: verify", err)
	}
	if err := s.store.MarkDeleting(ctx, f.ID); err != nil {
		return s.unexpected(ctx, "storage: verify", err)
	}
	return &UploadMismatchError{PublicID: f.PublicID, Reason: reason}
}

// MarkReady attaches a verified file; it joins the caller's unit of work.
func (s *Service) MarkReady(ctx context.Context, fileID uuid.UUID) error {
	if !s.Enabled() {
		return &NotConfiguredError{}
	}
	return s.errOrReport(ctx, "storage: mark ready", s.store.MarkReady(ctx, fileID, s.now()))
}

// MarkDeleting hands a removed file to the sweep; it joins the caller's unit of work.
func (s *Service) MarkDeleting(ctx context.Context, fileID uuid.UUID) error {
	if !s.Enabled() {
		return &NotConfiguredError{}
	}
	return s.errOrReport(ctx, "storage: mark deleting", s.store.MarkDeleting(ctx, fileID))
}

// Open streams a stored object; reading past maxBytes fails with *ObjectTooLargeError.
func (s *Service) Open(ctx context.Context, key string, maxBytes int64) (io.ReadCloser, error) {
	if !s.Enabled() {
		return nil, &NotConfiguredError{}
	}
	rc, err := s.driver.Open(ctx, key, maxBytes)
	if err != nil {
		return nil, s.unexpected(ctx, "storage: open", err)
	}
	return rc, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}

// File loads a file of the org on ctx by id, or *UploadNotFoundError.
func (s *Service) File(ctx context.Context, id uuid.UUID) (*File, error) {
	if !s.Enabled() {
		return nil, &NotConfiguredError{}
	}
	f, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "storage: file", err)
	}
	return f, nil
}
