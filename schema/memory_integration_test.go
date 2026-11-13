//go:build integration

package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestMemoryFacts_CascadeWithTheirStore(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	require.NoError(t, inserts(s)["memory_facts"](t.Context(), db, s.orgA, s.prjA))
	_, err := db.ExecContext(t.Context(), `DELETE FROM memory_stores WHERE project_id = $1`, s.prjA)
	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM memory_facts WHERE project_id = $1`, s.prjA).Scan(&n))
	assert.Zero(t, n, "a deleted store takes its facts")
}
