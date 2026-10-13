package legal

import (
	"errors"
	"fmt"
)

// UnknownDocumentError signals a slug with no embedded document.
type UnknownDocumentError struct{ Slug string }

func (e *UnknownDocumentError) Error() string {
	return fmt.Sprintf("legal: unknown document %q", e.Slug)
}

// IsUnknownDocumentError reports whether err wraps an *UnknownDocumentError.
func IsUnknownDocumentError(err error) bool {
	_, ok := errors.AsType[*UnknownDocumentError](err)
	return ok
}

// VersionChangedError reports a built-in document whose text changed while its version stayed the same.
type VersionChangedError struct{ Slug, Version string }

func (e *VersionChangedError) Error() string {
	return fmt.Sprintf("legal: the text of %s version %q changed; set a new updated: in content/%s.md", e.Slug, e.Version, e.Slug)
}
