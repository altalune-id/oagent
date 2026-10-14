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

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }

func (e *permanentError) Unwrap() error { return e.err }

func permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

func isPermanentError(err error) bool {
	_, ok := errors.AsType[*permanentError](err)
	return ok
}
