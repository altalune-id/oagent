package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sqlstateUniqueViolation     = "23505"
	sqlstateForeignKeyViolation = "23503"
)

// ErrTenantMismatch rejects joining a unit of work scoped to one org from a context scoped to another.
var ErrTenantMismatch = errors.New("db: unit of work joined with a different tenant scope")

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

// RLSError lists every reason a pool may not serve requests.
type RLSError struct {
	Pool    string
	Reasons []string
}

func (e *RLSError) Error() string {
	return fmt.Sprintf("db: the %s pool would not enforce row-level security: %s", e.Pool, strings.Join(e.Reasons, "; "))
}

// IsRLSError reports whether err's tree holds an *RLSError.
func IsRLSError(err error) bool {
	_, ok := errors.AsType[*RLSError](err)
	return ok
}
