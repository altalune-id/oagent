package db

import (
	"database/sql"
	"errors"

	"github.com/go-jet/jet/v2/qrm"
)

// IsNoRows reports whether err means a query found no row, from jet or database/sql.
func IsNoRows(err error) bool {
	return errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}
