package db

import (
	"errors"

	"altalune.id/oagent/pg"
)

// IsInvalidRoleError reports whether err wraps a pg.InvalidIdentifierError.
func IsInvalidRoleError(err error) bool {
	_, ok := errors.AsType[*pg.InvalidIdentifierError](err)
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
