//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
)

func setting(ctx context.Context, t *testing.T, q qrm.DB, name string) string {
	t.Helper()
	rows, err := q.QueryContext(ctx, `SELECT coalesce(current_setting($1, true), '')`, name)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var v string
	require.NoError(t, rows.Scan(&v))
	return v
}

func TestReadTuned_SettingsLastOnlyForTheRead(t *testing.T) {
	pool := singleConnPool(t)
	tune := []db.Setting{{Name: "hnsw.iterative_scan", Value: "relaxed_order"}, {Name: "hnsw.max_scan_tuples", Value: "100000"}}
	require.NoError(t, db.ReadTuned(t.Context(), pool, tune, func(q qrm.DB) error {
		assert.Equal(t, "relaxed_order", setting(t.Context(), t, q, "hnsw.iterative_scan"))
		assert.Equal(t, "100000", setting(t.Context(), t, q, "hnsw.max_scan_tuples"))
		return nil
	}))
	require.NoError(t, db.Read(t.Context(), pool, func(q qrm.DB) error {
		assert.Empty(t, setting(t.Context(), t, q, "hnsw.iterative_scan"), "gone after the call, on the same connection")
		assert.Empty(t, setting(t.Context(), t, q, "hnsw.max_scan_tuples"))
		return nil
	}))
}

func TestReadTuned_NeverLeaksIntoAnOuterUnitOfWork(t *testing.T) {
	pool := singleConnPool(t)
	require.NoError(t, db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
		err := db.ReadTuned(ctx, pool, []db.Setting{{Name: "hnsw.iterative_scan", Value: "relaxed_order"}}, func(qrm.DB) error { return nil })
		require.ErrorIs(t, err, db.ErrTunedInUnitOfWork)
		tx, _ := db.CurrentTx(ctx)
		assert.Empty(t, setting(ctx, t, tx, "hnsw.iterative_scan"), "the outer transaction never takes the setting")
		return nil
	}))
}
