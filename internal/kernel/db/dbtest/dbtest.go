// Package dbtest helps sqlmock tests expect the transactions kernel/db opens around every store call.
package dbtest

import "github.com/DATA-DOG/go-sqlmock"

const scopePattern = `SELECT set_config\('app\.current_org_id', \$1, true\),\s+` +
	`set_config\('app\.current_user_id', \$2, true\)`

// ExpectScope expects the tenant-scope statement kernel/db runs first in every transaction it opens.
func ExpectScope(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
	return mock.ExpectExec(scopePattern).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// ExpectTx expects kernel/db to begin a transaction and set its tenant scope.
func ExpectTx(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	ExpectScope(mock)
}
