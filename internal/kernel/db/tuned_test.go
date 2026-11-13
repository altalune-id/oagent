package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
)

func TestReadTuned_RefusesBeforeAnySQL(t *testing.T) {
	never := func(qrm.DB) error { return errors.New("fn must not run") }
	for name, tune := range map[string][]db.Setting{
		"unknown name":                {{Name: "work_mem", Value: "1GB"}},
		"tenant scope":                {{Name: "app.current_org_id", Value: "x"}},
		"bad iterative_scan":          {{Name: "hnsw.iterative_scan", Value: "on; DROP TABLE x"}},
		"zero max_scan_tuples":        {{Name: "hnsw.max_scan_tuples", Value: "0"}},
		"negative max_scan_tuples":    {{Name: "hnsw.max_scan_tuples", Value: "-5"}},
		"non-numeric max_scan_tuples": {{Name: "hnsw.max_scan_tuples", Value: "1e9"}},
		"padded max_scan_tuples":      {{Name: "hnsw.max_scan_tuples", Value: " 100"}},
		"one bad among good settings": {{Name: "hnsw.iterative_scan", Value: "relaxed_order"}, {Name: "hnsw.ef_search", Value: "40"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := db.ReadTuned(context.Background(), db.Pool{}, tune, never)
			require.ErrorIs(t, err, db.ErrSettingRefused)
		})
	}
}

func TestReadTuned_RefusesInsideAUnitOfWork(t *testing.T) {
	ctx := db.ContextWithTx(context.Background(), nil)
	err := db.ReadTuned(ctx, db.Pool{}, []db.Setting{{Name: "hnsw.iterative_scan", Value: "relaxed_order"}}, func(qrm.DB) error { return nil })
	require.ErrorIs(t, err, db.ErrTunedInUnitOfWork)
	assert.NotErrorIs(t, err, db.ErrSettingRefused)
}
