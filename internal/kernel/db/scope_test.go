package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
)

func expectScope(mock sqlmock.Sqlmock, tc tenant.Context) *sqlmock.ExpectedExec {
	return mock.ExpectExec(`SELECT set_config\('app\.current_org_id', \$1, true\),\s+set_config\('app\.current_user_id', \$2, true\)`).
		WithArgs(scopeArg(tc.OrgID), scopeArg(tc.UserID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func scopeArg(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func scoped() (context.Context, tenant.Context) {
	tc := tenant.Context{OrgID: uuid.New(), UserID: uuid.New()}
	return tenant.Into(context.Background(), tc), tc
}

func TestRunInTx_SetsTheScopeOncePerOutermostTransaction(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectCommit()

	require.NoError(t, RunInTx(ctx, pool, func(ctx context.Context) error {
		return RunInTx(ctx, pool, func(ctx context.Context) error {
			return InTx(ctx, pool, func(*sql.Tx) error { return nil })
		})
	}))
	assert.NoError(t, mock.ExpectationsWereMet(), "a joined unit of work must not set the scope again")
}

func TestRunInTx_ZeroIDsSetEmptyStrings(t *testing.T) {
	pool, mock := mockPool(t)
	tc := tenant.Context{OrgID: uuid.New()}
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectCommit()

	require.NoError(t, RunInTx(tenant.Into(context.Background(), tc), pool, func(context.Context) error { return nil }))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTx_ScopeErrorRollsBack(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc).WillReturnError(errors.New("no"))
	mock.ExpectRollback()

	called := false
	err := RunInTx(ctx, pool, func(context.Context) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db: set tenant scope")
	assert.False(t, called, "fn must not run without its scope")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestJoin_RefusesADifferentOrg(t *testing.T) {
	ctx, tc := scoped()
	joins := map[string]func(ctx context.Context, pool Pool) error{
		"RunInTx": func(ctx context.Context, pool Pool) error {
			return RunInTx(ctx, pool, func(context.Context) error { return nil })
		},
		"InTx": func(ctx context.Context, pool Pool) error {
			return InTx(ctx, pool, func(*sql.Tx) error { return nil })
		},
		"Read": func(ctx context.Context, pool Pool) error {
			return Read(ctx, pool, func(qrm.DB) error { return nil })
		},
	}
	for name, join := range joins {
		t.Run(name, func(t *testing.T) {
			pool, mock := mockPool(t)
			mock.ExpectBegin()
			expectScope(mock, tc).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectRollback()

			err := RunInTx(ctx, pool, func(ctx context.Context) error { return join(tenant.WithOrg(ctx, uuid.New()), pool) })
			require.ErrorIs(t, err, ErrTenantMismatch)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestJoin_SetsTheOrgUnderAnUnscopedOuter(t *testing.T) {
	for name, join := range joins() {
		t.Run(name, func(t *testing.T) {
			pool, mock := mockPool(t)
			org := uuid.New()
			mock.ExpectBegin()
			expectScope(mock, tenant.Context{})
			expectOrg(mock, org)
			mock.ExpectCommit()

			require.NoError(t, RunInTx(context.Background(), pool, func(ctx context.Context) error {
				if err := join(tenant.WithOrg(ctx, org), pool); err != nil {
					return err
				}
				if err := join(tenant.WithOrg(ctx, org), pool); err != nil {
					return err
				}
				return join(ctx, pool)
			}))
			assert.NoError(t, mock.ExpectationsWereMet(), "the org is set once and then kept")
		})
	}
}

func TestJoin_ThenRefusesAnotherOrg(t *testing.T) {
	pool, mock := mockPool(t)
	org := uuid.New()
	mock.ExpectBegin()
	expectScope(mock, tenant.Context{})
	expectOrg(mock, org)
	mock.ExpectRollback()

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		if err := Read(tenant.WithOrg(ctx, org), pool, func(qrm.DB) error { return nil }); err != nil {
			return err
		}
		return Read(tenant.WithOrg(ctx, uuid.New()), pool, func(qrm.DB) error { return nil })
	})
	require.ErrorIs(t, err, ErrTenantMismatch)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestJoin_OrgErrorFailsTheJoin(t *testing.T) {
	pool, mock := mockPool(t)
	org := uuid.New()
	mock.ExpectBegin()
	expectScope(mock, tenant.Context{})
	expectOrg(mock, org).WillReturnError(errors.New("no"))
	mock.ExpectRollback()

	err := RunInTx(context.Background(), pool, func(ctx context.Context) error {
		return InTx(tenant.WithOrg(ctx, org), pool, func(*sql.Tx) error { return nil })
	})
	require.ErrorContains(t, err, "db: set org scope")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestJoin_AcceptsTheSameOrgOrNone(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectCommit()

	require.NoError(t, RunInTx(ctx, pool, func(ctx context.Context) error {
		if err := Read(tenant.WithOrg(ctx, tc.OrgID), pool, func(qrm.DB) error { return nil }); err != nil {
			return err
		}
		return Read(tenant.Into(ctx, tenant.Context{}), pool, func(qrm.DB) error { return nil })
	}), "a context without a scope keeps the org")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRead_OwnsAReadOnlyTransactionOnTheReader(t *testing.T) {
	pool, wmock, rmock := mockPools(t)
	ctx, tc := scoped()
	rmock.ExpectBegin()
	expectScope(rmock, tc)
	rmock.ExpectQuery(`SELECT 1`).WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))
	rmock.ExpectCommit()

	require.NoError(t, Read(ctx, pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, `SELECT 1`)
		if err != nil {
			return err
		}
		return rows.Close()
	}))
	assert.NoError(t, rmock.ExpectationsWereMet())
	assert.NoError(t, wmock.ExpectationsWereMet(), "Read must not touch the writer")
}

func TestRead_RollsBackOnError(t *testing.T) {
	pool, _, rmock := mockPools(t)
	rmock.ExpectBegin()
	expectScope(rmock, tenant.Context{})
	rmock.ExpectRollback()
	boom := errors.New("boom")

	require.ErrorIs(t, Read(context.Background(), pool, func(qrm.DB) error { return boom }), boom)
	assert.NoError(t, rmock.ExpectationsWereMet())
}

func TestRead_JoinsTheAmbientTransaction(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	expectScope(mock, tenant.Context{})
	mock.ExpectCommit()

	require.NoError(t, RunInTx(context.Background(), pool, func(ctx context.Context) error {
		outer, _ := CurrentTx(ctx)
		return Read(ctx, pool, func(q qrm.DB) error {
			assert.Same(t, outer, q, "Read must join the unit of work on ctx")
			return nil
		})
	}))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRead_NilReader(t *testing.T) {
	pool, _ := mockPool(t)
	pool.R = nil
	assert.Error(t, Read(context.Background(), pool, func(qrm.DB) error { return nil }))
}

func TestContextWithTx_JoinsAsUnscoped(t *testing.T) {
	pool, mock := mockPool(t)
	org := uuid.New()
	mock.ExpectBegin()
	expectOrg(mock, org)
	mock.ExpectRollback()
	tx, err := pool.W.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	ctx := ContextWithTx(context.Background(), tx)

	assert.NoError(t, Read(ctx, Pool{}, func(qrm.DB) error { return nil }), "a caller-owned transaction carries no scope")
	assert.NoError(t, Read(tenant.WithOrg(ctx, org), Pool{}, func(qrm.DB) error { return nil }), "a scoped join sets the org on it")
	require.NoError(t, tx.Rollback())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWrite_OwnsAScopedTransactionOnTheWriter(t *testing.T) {
	pool, wmock, rmock := mockPools(t)
	ctx, tc := scoped()
	wmock.ExpectBegin()
	expectScope(wmock, tc)
	wmock.ExpectExec(`DELETE`).WillReturnResult(sqlmock.NewResult(0, 1))
	wmock.ExpectCommit()

	require.NoError(t, Write(ctx, pool, func(q qrm.DB) error {
		_, err := q.ExecContext(ctx, `DELETE`)
		return err
	}))
	assert.NoError(t, wmock.ExpectationsWereMet())
	assert.NoError(t, rmock.ExpectationsWereMet(), "Write must not touch the reader")
}

func TestWrite_JoinsTheAmbientTransactionOrRefusesAnotherOrg(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectRollback()

	err := RunInTx(ctx, pool, func(ctx context.Context) error {
		outer, _ := CurrentTx(ctx)
		require.NoError(t, Write(ctx, pool, func(q qrm.DB) error {
			assert.Same(t, outer, q)
			return nil
		}))
		return Write(tenant.WithOrg(ctx, uuid.New()), pool, func(qrm.DB) error { return nil })
	})
	require.ErrorIs(t, err, ErrTenantMismatch)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReadPrimary_OwnsAReadOnlyTransactionOnTheWriter(t *testing.T) {
	pool, wmock, rmock := mockPools(t)
	ctx, tc := scoped()
	wmock.ExpectBegin()
	expectScope(wmock, tc)
	wmock.ExpectQuery(`SELECT 1`).WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))
	wmock.ExpectCommit()

	require.NoError(t, ReadPrimary(ctx, pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, `SELECT 1`)
		if err != nil {
			return err
		}
		return rows.Close()
	}))
	assert.NoError(t, wmock.ExpectationsWereMet())
	assert.NoError(t, rmock.ExpectationsWereMet(), "ReadPrimary must not touch the reader")
}

func TestReadPrimary_JoinsTheAmbientTransactionOrRefusesAnotherOrg(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectRollback()

	err := RunInTx(ctx, pool, func(ctx context.Context) error {
		outer, _ := CurrentTx(ctx)
		require.NoError(t, ReadPrimary(ctx, pool, func(q qrm.DB) error {
			assert.Same(t, outer, q)
			return nil
		}))
		return ReadPrimary(tenant.WithOrg(ctx, uuid.New()), pool, func(qrm.DB) error { return nil })
	})
	require.ErrorIs(t, err, ErrTenantMismatch)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func expectOrg(mock sqlmock.Sqlmock, org uuid.UUID) *sqlmock.ExpectedExec {
	return mock.ExpectExec(`SELECT set_config\('app\.current_org_id', \$1, true\)`).
		WithArgs(scopeArg(org)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func joins() map[string]func(ctx context.Context, pool Pool) error {
	return map[string]func(ctx context.Context, pool Pool) error{
		"RunInTx": func(ctx context.Context, pool Pool) error {
			return RunInTx(ctx, pool, func(context.Context) error { return nil })
		},
		"InTx": func(ctx context.Context, pool Pool) error { return InTx(ctx, pool, func(*sql.Tx) error { return nil }) },
		"Read": func(ctx context.Context, pool Pool) error { return Read(ctx, pool, func(qrm.DB) error { return nil }) },
	}
}

func TestJoin_KeepsTheOutermostOrgWhenTheContextNamesIt(t *testing.T) {
	pool, mock := mockPool(t)
	ctx, tc := scoped()
	mock.ExpectBegin()
	expectScope(mock, tc)
	mock.ExpectCommit()

	require.NoError(t, RunInTx(ctx, pool, func(ctx context.Context) error {
		return Read(tenant.WithOrg(ctx, tc.OrgID), pool, func(qrm.DB) error { return nil })
	}))
	assert.NoError(t, mock.ExpectationsWereMet())
}
