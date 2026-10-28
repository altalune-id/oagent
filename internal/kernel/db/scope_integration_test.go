//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func singleConnPool(t *testing.T) db.Pool {
	t.Helper()
	sqlDB := pgtest.New(t).OpenDB(t)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return db.Pool{W: sqlDB, R: sqlDB}
}

type settings struct{ org, user string }

func readSettings(ctx context.Context, t *testing.T, q qrm.DB) settings {
	t.Helper()
	rows, err := q.QueryContext(ctx, `SELECT coalesce(current_setting('app.current_org_id', true), ''),
		coalesce(current_setting('app.current_user_id', true), '')`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var s settings
	require.NoError(t, rows.Scan(&s.org, &s.user))
	require.NoError(t, rows.Err())
	return s
}

func TestScope_IsSetInsideEveryUnitOfWork(t *testing.T) {
	pool := singleConnPool(t)
	tc := tenant.Context{OrgID: uuid.New(), UserID: uuid.New()}
	ctx := tenant.Into(t.Context(), tc)
	want := settings{tc.OrgID.String(), tc.UserID.String()}

	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		assert.Equal(t, want, readSettings(ctx, t, tx))
		return nil
	}))
	require.NoError(t, db.Read(ctx, pool, func(q qrm.DB) error {
		assert.Equal(t, want, readSettings(ctx, t, q))
		return nil
	}))
}

func TestScope_DiesWithTheTransactionOnTheSameConnection(t *testing.T) {
	pool := singleConnPool(t)
	ctx := tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New(), UserID: uuid.New()})

	require.NoError(t, db.Read(ctx, pool, func(qrm.DB) error { return nil }))

	assert.Equal(t, settings{}, readSettings(t.Context(), t, pool.R),
		"after commit the same pooled connection must carry no tenant: the scope is transaction-local")
	require.NoError(t, db.Read(t.Context(), pool, func(q qrm.DB) error {
		assert.Equal(t, settings{}, readSettings(t.Context(), t, q), "an unscoped unit of work sets every id to empty")
		return nil
	}))
}

func TestRead_IsReadOnly(t *testing.T) {
	pool := singleConnPool(t)
	err := db.Read(t.Context(), pool, func(q qrm.DB) error {
		_, err := q.ExecContext(t.Context(), `CREATE TABLE read_only_probe (v int)`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only transaction")
}

func TestReadPrimary_IsReadOnlyOnTheWriter(t *testing.T) {
	w := singleConnPool(t)
	r := singleConnPool(t)
	pool := db.Pool{W: w.W, R: r.R}
	_, err := w.W.ExecContext(t.Context(), `CREATE TABLE primary_probe (v int)`)
	require.NoError(t, err)

	require.NoError(t, db.ReadPrimary(t.Context(), pool, func(q qrm.DB) error {
		_, err := q.ExecContext(t.Context(), `SELECT count(*) FROM primary_probe`)
		return err
	}), "ReadPrimary must run on the writer, where the table exists")
	err = db.ReadPrimary(t.Context(), pool, func(q qrm.DB) error {
		_, err := q.ExecContext(t.Context(), `INSERT INTO primary_probe VALUES (1)`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only transaction")
}
