// Package knowledge holds a project's knowledge sources, their documents and the chunks agents search.
package knowledge

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/weburl"
)

// SourceNotFoundError reports a knowledge source that does not exist or that the caller does not reach.
type SourceNotFoundError struct{}

func (*SourceNotFoundError) Error() string { return "knowledge: source not found" }

// IsSourceNotFoundError reports whether err's tree contains a *SourceNotFoundError.
func IsSourceNotFoundError(err error) bool {
	_, ok := errors.AsType[*SourceNotFoundError](err)
	return ok
}

// ToAppError maps SourceNotFoundError to its own code.
func (e *SourceNotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeSourceNotFound, "Knowledge source not found").WithCause(e)
}

// SourceReason is the stable cause of an InvalidSourceError, sent to clients as meta.reason.
type SourceReason string

// The causes of an InvalidSourceError.
const (
	SourceURLRequired       = SourceReason(weburl.Required)
	SourceURLTooLong        = SourceReason(weburl.TooLong)
	SourceURLNotUTF8        = SourceReason(weburl.NotUTF8)
	SourceURLUnparsable     = SourceReason(weburl.Unparsable)
	SourceURLHTTPSRequired  = SourceReason(weburl.HTTPSRequired)
	SourceURLHasCredentials = SourceReason(weburl.HasCredentials)
	SourceURLNoHost         = SourceReason(weburl.NoHost)
	SourceBackendDisabled   = SourceReason("backend_disabled")
	SourceKindMismatch      = SourceReason("kind")
)

func (r SourceReason) text() string {
	if r == SourceKindMismatch {
		return "kind does not allow this action"
	}
	if r == SourceBackendDisabled {
		return "backend is not enabled on this server"
	}
	return "URL " + weburl.Reason(r).Text()
}

// InvalidSourceError reports a source whose seed URL or backend breaks its rules.
type InvalidSourceError struct{ Reason SourceReason }

func (e *InvalidSourceError) Error() string { return "knowledge: source " + e.Reason.text() }

// IsInvalidSourceError reports whether err's tree contains an *InvalidSourceError.
func IsInvalidSourceError(err error) bool {
	_, ok := errors.AsType[*InvalidSourceError](err)
	return ok
}

// ToAppError maps InvalidSourceError to its own code with the reason and the URL length limit.
func (e *InvalidSourceError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeSourceInvalid, "Knowledge source "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(weburl.MaxLen)}).WithCause(e)
}

// SourceAlreadyExistsError reports a source name already taken in the project.
type SourceAlreadyExistsError struct{ Name string }

func (e *SourceAlreadyExistsError) Error() string {
	return "knowledge: source name already taken: " + apperror.Echo(e.Name)
}

// IsSourceAlreadyExistsError reports whether err's tree contains a *SourceAlreadyExistsError.
func IsSourceAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*SourceAlreadyExistsError](err)
	return ok
}

// ToAppError maps SourceAlreadyExistsError to its own code with the name.
func (e *SourceAlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeSourceAlreadyExists, "Knowledge source name already taken").
		WithMeta(map[string]string{"name": apperror.Echo(e.Name)}).WithCause(e)
}

// SourceInUseError reports a source that a published agent version uses.
type SourceInUseError struct{}

func (*SourceInUseError) Error() string {
	return "knowledge: source is used by a published agent version"
}

// IsSourceInUseError reports whether err's tree contains a *SourceInUseError.
func IsSourceInUseError(err error) bool {
	_, ok := errors.AsType[*SourceInUseError](err)
	return ok
}

// ToAppError maps SourceInUseError to its own code.
func (e *SourceInUseError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeSourceInUse, "Knowledge source is used by a published agent version").WithCause(e)
}

// DocumentNotFoundError reports a document that does not exist or that the caller does not reach.
type DocumentNotFoundError struct{}

func (*DocumentNotFoundError) Error() string { return "knowledge: document not found" }

// IsDocumentNotFoundError reports whether err's tree contains a *DocumentNotFoundError.
func IsDocumentNotFoundError(err error) bool {
	_, ok := errors.AsType[*DocumentNotFoundError](err)
	return ok
}

// ToAppError maps DocumentNotFoundError to its own code.
func (e *DocumentNotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeDocumentNotFound, "Document not found").WithCause(e)
}

// RejectReason is the stable cause of an UploadRejectedError, sent to clients as meta.reason.
type RejectReason string

// The causes of an UploadRejectedError.
const (
	RejectMissing            RejectReason = "missing"
	RejectSize               RejectReason = "size"
	RejectType               RejectReason = "type"
	RejectContent            RejectReason = "content"
	RejectExpired            RejectReason = "expired"
	RejectTooLarge           RejectReason = "too_large"
	RejectTooManyEntries     RejectReason = "too_many_entries"
	RejectTooManyPages       RejectReason = "too_many_pages"
	RejectMissingDocumentXML RejectReason = "missing_document_xml"
	RejectInvalidUTF8        RejectReason = "invalid_utf8"
	RejectUnsupportedType    RejectReason = "unsupported_type"
)

// UploadRejectedError reports a file that failed verification or a format check.
type UploadRejectedError struct{ Reason RejectReason }

func (e *UploadRejectedError) Error() string { return "knowledge: file rejected: " + string(e.Reason) }

// IsUploadRejectedError reports whether err's tree contains an *UploadRejectedError.
func IsUploadRejectedError(err error) bool {
	_, ok := errors.AsType[*UploadRejectedError](err)
	return ok
}

// ToAppError maps UploadRejectedError to its own code with the reason.
func (e *UploadRejectedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeUploadRejected, "The file was rejected").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// NoTextFoundError reports a document with no extractable text, such as a scanned PDF.
type NoTextFoundError struct{}

func (*NoTextFoundError) Error() string { return "knowledge: no text found" }

// IsNoTextFoundError reports whether err's tree contains a *NoTextFoundError.
func IsNoTextFoundError(err error) bool {
	_, ok := errors.AsType[*NoTextFoundError](err)
	return ok
}

// ToAppError maps NoTextFoundError to its own code.
func (e *NoTextFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeNoTextFound, "No text was found in this file").WithCause(e)
}

// UnreadableReason is the stable cause of an UnreadableError, sent to clients as meta.reason.
type UnreadableReason string

// The causes of an UnreadableError.
const (
	UnreadableCorrupt        UnreadableReason = "corrupt"
	UnreadableTimeout        UnreadableReason = "timeout"
	UnreadableOutputOverflow UnreadableReason = "output_overflow"
	UnreadableCrashed        UnreadableReason = "crashed"
)

// UnreadableError reports a file the extractor could not read.
type UnreadableError struct{ Reason UnreadableReason }

func (e *UnreadableError) Error() string {
	return "knowledge: could not read file: " + string(e.Reason)
}

// IsUnreadableError reports whether err's tree contains an *UnreadableError.
func IsUnreadableError(err error) bool {
	_, ok := errors.AsType[*UnreadableError](err)
	return ok
}

// ToAppError maps UnreadableError to its own code with the reason.
func (e *UnreadableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeCouldNotReadFile, "The file could not be read").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// EmbeddingReason is the stable cause of an EmbeddingModelInvalidError, sent to clients as meta.reason.
type EmbeddingReason string

// The causes of an EmbeddingModelInvalidError.
const (
	EmbeddingSize         EmbeddingReason = "size"
	EmbeddingProviderKind EmbeddingReason = "provider_kind"
	EmbeddingNotEmbedding EmbeddingReason = "not_embedding"
	EmbeddingUnpriced     EmbeddingReason = "unpriced"
)

// EmbeddingModelInvalidError reports a model that cannot be the project's embedding model; Dims is the probed size, for EmbeddingSize.
type EmbeddingModelInvalidError struct {
	Reason EmbeddingReason
	Dims   int
}

func (e *EmbeddingModelInvalidError) Error() string {
	if e.Reason == EmbeddingSize {
		return "knowledge: unsupported embedding size " + strconv.Itoa(e.Dims)
	}
	return "knowledge: embedding model invalid: " + string(e.Reason)
}

// IsEmbeddingModelInvalidError reports whether err's tree contains an *EmbeddingModelInvalidError.
func IsEmbeddingModelInvalidError(err error) bool {
	_, ok := errors.AsType[*EmbeddingModelInvalidError](err)
	return ok
}

// ToAppError maps EmbeddingModelInvalidError to its own code with the reason and, for a size, the dimensions.
func (e *EmbeddingModelInvalidError) ToAppError() *apperror.AppError {
	meta := map[string]string{"reason": string(e.Reason)}
	if e.Reason == EmbeddingSize {
		meta["dims"] = strconv.Itoa(e.Dims)
	}
	return apperror.New(apperror.CodeKnowledgeEmbeddingModelInvalid, "This embedding model can't be used").
		WithMeta(meta).WithCause(e)
}

// EmbeddingModelNotSetError reports a project with no embedding model.
type EmbeddingModelNotSetError struct{}

func (*EmbeddingModelNotSetError) Error() string { return "knowledge: no embedding model set" }

// IsEmbeddingModelNotSetError reports whether err's tree contains an *EmbeddingModelNotSetError.
func IsEmbeddingModelNotSetError(err error) bool {
	_, ok := errors.AsType[*EmbeddingModelNotSetError](err)
	return ok
}

// ToAppError maps EmbeddingModelNotSetError to its own code.
func (e *EmbeddingModelNotSetError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeEmbeddingModelNotSet, "The project has no embedding model").WithCause(e)
}

// EmbedFailReason is the stable cause of an EmbeddingFailedError, sent to clients as meta.reason.
type EmbedFailReason string

// The causes of an EmbeddingFailedError.
const (
	EmbeddingUnreachable EmbedFailReason = "unreachable"
	EmbeddingRejected    EmbedFailReason = "rejected"
	EmbeddingWrongDims   EmbedFailReason = "wrong_dims"
)

// EmbeddingFailedError reports an embeddings call that failed or answered vectors of another size.
type EmbeddingFailedError struct {
	Reason EmbedFailReason
	Cause  error
}

func (e *EmbeddingFailedError) Error() string {
	if e.Cause == nil {
		return "knowledge: embedding failed: " + string(e.Reason)
	}
	return "knowledge: embedding failed: " + string(e.Reason) + ": " + e.Cause.Error()
}

func (e *EmbeddingFailedError) Unwrap() error { return e.Cause }

// IsEmbeddingFailedError reports whether err's tree contains an *EmbeddingFailedError.
func IsEmbeddingFailedError(err error) bool {
	_, ok := errors.AsType[*EmbeddingFailedError](err)
	return ok
}

// ToAppError maps EmbeddingFailedError to its own code with the reason; the cause stays server-side.
func (e *EmbeddingFailedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeEmbeddingFailed, "The embedding model failed").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// StorageNotConfiguredError reports an upload on a server with no file storage.
type StorageNotConfiguredError struct{}

func (*StorageNotConfiguredError) Error() string { return "knowledge: file storage is not configured" }

// IsStorageNotConfiguredError reports whether err's tree contains a *StorageNotConfiguredError.
func IsStorageNotConfiguredError(err error) bool {
	_, ok := errors.AsType[*StorageNotConfiguredError](err)
	return ok
}

// ToAppError maps StorageNotConfiguredError to its own code.
func (e *StorageNotConfiguredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeStorageNotConfigured, "File storage is not configured on this server").WithCause(e)
}

// LimitReason is the stable cause of a LimitReachedError, sent to clients as meta.reason.
type LimitReason string

// The causes of a LimitReachedError.
const (
	LimitDocumentsPerSource LimitReason = "documents_per_source"
	LimitChunksPerProject   LimitReason = "chunks_per_project"
	LimitTextPerDocument    LimitReason = "text_per_document"
	LimitChunksPerDocument  LimitReason = "chunks_per_document"
	LimitSourcesPerProject  LimitReason = "sources_per_project"
	LimitRefreshTooSoon     LimitReason = "refresh_too_soon"
)

// LimitReachedError reports a knowledge cap that refuses more; Max is the cap.
type LimitReachedError struct {
	Reason LimitReason
	Max    int
}

func (e *LimitReachedError) Error() string {
	return "knowledge: limit reached: " + string(e.Reason) + " max " + strconv.Itoa(e.Max)
}

// IsLimitReachedError reports whether err's tree contains a *LimitReachedError.
func IsLimitReachedError(err error) bool {
	_, ok := errors.AsType[*LimitReachedError](err)
	return ok
}

// ToAppError maps LimitReachedError to its own code with the reason and the cap.
func (e *LimitReachedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeLimitReached, "A knowledge limit was reached").
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(e.Max)}).WithCause(e)
}

// CrawlReason is the stable cause of a CrawlFailedError, sent to clients as meta.reason.
type CrawlReason string

// The causes of a CrawlFailedError.
const (
	CrawlOffHostRedirect  CrawlReason = "off_host_redirect"
	CrawlRobotsDisallowed CrawlReason = "robots_disallowed"
	CrawlUnreachable      CrawlReason = "unreachable"
	CrawlNoPages          CrawlReason = "no_pages"
)

// CrawlFailedError reports a URL source whose crawl found nothing it may read.
type CrawlFailedError struct{ Reason CrawlReason }

func (e *CrawlFailedError) Error() string { return "knowledge: crawl failed: " + string(e.Reason) }

// IsCrawlFailedError reports whether err's tree contains a *CrawlFailedError.
func IsCrawlFailedError(err error) bool {
	_, ok := errors.AsType[*CrawlFailedError](err)
	return ok
}

// ToAppError maps CrawlFailedError to its own code with the reason.
func (e *CrawlFailedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeKnowledgeCrawlFailed, "The site could not be read").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// IsExpectedError reports whether err is a typed knowledge error, or any error that carries its own code, a service returns as-is.
func IsExpectedError(err error) bool {
	if _, ok := apperror.AsAppError(err); ok {
		return true
	}
	return IsSourceNotFoundError(err) || IsInvalidSourceError(err) || IsSourceAlreadyExistsError(err) || IsSourceInUseError(err) ||
		IsDocumentNotFoundError(err) || IsUploadRejectedError(err) || IsNoTextFoundError(err) || IsUnreadableError(err) ||
		IsEmbeddingModelInvalidError(err) || IsEmbeddingModelNotSetError(err) || IsEmbeddingFailedError(err) ||
		IsStorageNotConfiguredError(err) || IsLimitReachedError(err) || IsCrawlFailedError(err)
}
