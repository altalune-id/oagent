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
