package db

import (
	"database/sql"
	"errors"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sqlstateUniqueViolation     = "23505"
	sqlstateForeignKeyViolation = "23503"
)

// IsNoRows reports whether err means a query found no row, from jet or database/sql.
func IsNoRows(err error) bool {
	return errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

// IsUniqueViolation reports whether err's tree holds a unique violation.
func IsUniqueViolation(err error) bool { return hasCode(err, sqlstateUniqueViolation) }

// IsForeignKeyViolation reports whether err's tree holds a foreign-key violation.
func IsForeignKeyViolation(err error) bool { return hasCode(err, sqlstateForeignKeyViolation) }

// ViolatedConstraint returns the name of the constraint a database error in err's tree names, if any.
func ViolatedConstraint(err error) (string, bool) {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.ConstraintName == "" {
		return "", false
	}
	return pgErr.ConstraintName, true
}

func hasCode(err error, code string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == code
}
