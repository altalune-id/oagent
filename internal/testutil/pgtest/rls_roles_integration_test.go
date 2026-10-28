//go:build integration

package pgtest

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureRLSRoles_IsIdempotentAndRepairsAttributes(t *testing.T) {
	admin := New(t).OpenDB(t)
	owner, service := scratchRoles(t, admin)
	require.NoError(t, ensureRLSRoles(t.Context(), admin, owner, service))

	for _, stmt := range []string{
		`ALTER ROLE ` + service + ` BYPASSRLS CREATEROLE`,
		`ALTER ROLE ` + owner + ` NOBYPASSRLS`,
		`GRANT ` + owner + ` TO ` + service,
	} {
		_, err := admin.ExecContext(t.Context(), stmt)
		require.NoError(t, err, stmt)
	}

	require.NoError(t, ensureRLSRoles(t.Context(), admin, owner, service))

	var svcBypass, svcCreateRole, svcLogin, ownerBypass, ownerLogin, member bool
	require.NoError(t, admin.QueryRowContext(t.Context(), `
		SELECT s.rolbypassrls, s.rolcreaterole, s.rolcanlogin, o.rolbypassrls, o.rolcanlogin,
		       pg_has_role(s.rolname, o.rolname, 'MEMBER')
		  FROM pg_roles s, pg_roles o WHERE s.rolname = $1 AND o.rolname = $2`,
		service, owner).Scan(&svcBypass, &svcCreateRole, &svcLogin, &ownerBypass, &ownerLogin, &member))
	assert.False(t, svcBypass)
	assert.False(t, svcCreateRole)
	assert.True(t, svcLogin)
	assert.True(t, ownerBypass, "the owner keeps BYPASSRLS for the definer functions it owns")
	assert.False(t, ownerLogin)
	assert.False(t, member, "a re-run removes any membership the service role picked up")
}

func TestEnsureRLSRoles_ConcurrentCallersSerialize(t *testing.T) {
	admin := New(t).OpenDB(t)
	owner, service := scratchRoles(t, admin)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() { errs[i] = ensureRLSRoles(t.Context(), admin, owner, service) })
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err, "the advisory lock must serialize role creation across callers")
	}
}

func scratchRoles(t *testing.T, admin *sql.DB) (owner, service string) {
	t.Helper()
	suffix := strings.TrimPrefix(uniqueSchema(t), schemaPrefix)
	owner, service = "pgt_owner_"+suffix, "pgt_service_"+suffix
	t.Cleanup(func() {
		for _, role := range []string{service, owner} {
			if _, err := admin.ExecContext(context.Background(), `DROP OWNED BY `+role+`; DROP ROLE `+role); err != nil {
				t.Errorf("pgtest: leaked role %s: %v", role, err)
			}
		}
	})
	return owner, service
}
