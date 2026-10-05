package pg

import (
	"errors"
	"fmt"
)

// InvalidIdentifierError reports a name that is not usable as a Postgres identifier.
type InvalidIdentifierError struct {
	Name   string
	Reason string
}

func (e *InvalidIdentifierError) Error() string {
	return fmt.Sprintf("pg: invalid identifier %q: %s", e.Name, e.Reason)
}

// IsInvalidIdentifierError reports whether err wraps an *InvalidIdentifierError.
func IsInvalidIdentifierError(err error) bool {
	_, ok := errors.AsType[*InvalidIdentifierError](err)
	return ok
}
