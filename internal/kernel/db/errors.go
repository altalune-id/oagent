package db

import (
	"database/sql"
	"errors"

	"github.com/go-jet/jet/v2/qrm"

	"altalune.id/oagent/pg"
)

// IsNoRows reports whether err means a query found no row, from jet or database/sql.
func IsNoRows(err error) bool {
	return errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

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
