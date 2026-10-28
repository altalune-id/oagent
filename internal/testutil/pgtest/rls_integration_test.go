//go:build integration

package pgtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestMigratedRLS_ServiceRoleCannotBypassRLS(t *testing.T) {
	svc, _, _ := pgtest.MigratedRLS(t)

	var user string
	var bypass, super, reachesOwner bool
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `
		SELECT current_user, r.rolbypassrls, r.rolsuper, pg_has_role(current_user, $1, 'MEMBER')
		  FROM pg_roles r WHERE r.rolname = current_user`, pgtest.RLSOwnerRole).
		Scan(&user, &bypass, &super, &reachesOwner))
	assert.Equal(t, pgtest.RLSServiceRole, user)
	assert.False(t, bypass, "the service role must be NOBYPASSRLS, or RLS tests prove nothing")
	assert.False(t, super)
	assert.False(t, reachesOwner, "the service role must not reach the owner by SET ROLE")
	assert.Same(t, svc.W, svc.R, "reads and writes share the service connection")
}

func TestMigratedRLS_OwnerOwnsTheSchemaAndTheServiceOnlyGetsDML(t *testing.T) {
	svc, admin, schemaName := pgtest.MigratedRLS(t)

	var notOwned int
	require.NoError(t, admin.QueryRowContext(t.Context(), `
		SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relkind IN ('r', 'p', 'S') AND pg_get_userbyid(c.relowner) <> $2`,
		schemaName, pgtest.RLSOwnerRole).Scan(&notOwned))
	assert.Zero(t, notOwned, "migrations must run as the owner")

	var dml, gooseRead, gooseWrite, create bool
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `
		SELECT has_table_privilege('orgs', 'SELECT, INSERT, UPDATE, DELETE'),
		       has_table_privilege('goose_db_version', 'SELECT'),
		       has_table_privilege('goose_db_version', 'INSERT, UPDATE, DELETE, TRUNCATE'),
		       has_schema_privilege($1, 'CREATE')`, schemaName).Scan(&dml, &gooseRead, &gooseWrite, &create))
	assert.True(t, dml)
	assert.True(t, gooseRead)
	assert.False(t, gooseWrite, "the service role must not rewrite migration history")
	assert.False(t, create, "the service role must not create objects")

	var n int
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `SELECT count(*) FROM orgs`).Scan(&n))
	assert.Zero(t, n)
}

func TestMigratedRLS_SchemasStayApart(t *testing.T) {
	svcA, _, schemaA := pgtest.MigratedRLS(t)
	svcB, adminB, schemaB := pgtest.MigratedRLS(t)
	require.NotEqual(t, schemaA, schemaB)

	var pathA, pathB, adminPath string
	require.NoError(t, svcA.W.QueryRowContext(t.Context(), `SELECT current_schema()`).Scan(&pathA))
	require.NoError(t, svcB.W.QueryRowContext(t.Context(), `SELECT current_schema()`).Scan(&pathB))
	require.NoError(t, adminB.QueryRowContext(t.Context(), `SELECT current_schema()`).Scan(&adminPath))
	assert.Equal(t, schemaA, pathA)
	assert.Equal(t, schemaB, pathB)
	assert.Equal(t, schemaB, adminPath, "the admin handle seeds the same schema the service reads")
}
