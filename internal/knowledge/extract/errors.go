package extract

import "errors"

// RejectReason is the stable cause of a RejectedError.
type RejectReason string

// The causes of a RejectedError.
const (
	TooLarge           RejectReason = "too_large"
	TooManyEntries     RejectReason = "too_many_entries"
	TooManyPages       RejectReason = "too_many_pages"
	MissingDocumentXML RejectReason = "missing_document_xml"
	InvalidUTF8        RejectReason = "invalid_utf8"
	UnsupportedType    RejectReason = "unsupported_type"
)

// RejectedError reports that the file breaks a format or size rule before or while it is read.
type RejectedError struct{ Reason RejectReason }

func (e *RejectedError) Error() string { return "extract: file rejected: " + string(e.Reason) }

// AsRejectedError returns the *RejectedError in err's tree, if any.
func AsRejectedError(err error) (*RejectedError, bool) {
	return errors.AsType[*RejectedError](err)
}

// NoTextError reports that the file holds no extractable text.
type NoTextError struct{}

func (*NoTextError) Error() string { return "extract: no text found" }

// IsNoTextError reports whether err's tree contains a *NoTextError.
func IsNoTextError(err error) bool {
	_, ok := errors.AsType[*NoTextError](err)
	return ok
}

// UnreadReason is the stable cause of an UnreadableError.
type UnreadReason string

// The causes of an UnreadableError.
const (
	Corrupt  UnreadReason = "corrupt"
	Timeout  UnreadReason = "timeout"
	Overflow UnreadReason = "output_overflow"
	Crashed  UnreadReason = "crashed"
)

// UnreadableError reports that the file could not be read: corrupt, or the extractor timed out, crashed or overflowed.
type UnreadableError struct {
	Reason UnreadReason
	Cause  error
}

func (e *UnreadableError) Error() string {
	if e.Cause == nil {
		return "extract: could not read file: " + string(e.Reason)
	}
	return "extract: could not read file: " + string(e.Reason) + ": " + e.Cause.Error()
}

func (e *UnreadableError) Unwrap() error { return e.Cause }

// IsUnreadableError reports whether err's tree contains an *UnreadableError.
func IsUnreadableError(err error) bool {
	_, ok := errors.AsType[*UnreadableError](err)
	return ok
}
