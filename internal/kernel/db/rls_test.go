package db

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/pg"
)

func cleanReport() pg.RLSReport {
	return pg.RLSReport{
		SessionUser: "oagent_service", CurrentUser: "oagent_service", RowSecurity: "on",
		Tables: []pg.TableRLS{
			{Name: "orgs", Enabled: true, Force: true, ScopedRead: true, WriteCovered: true},
			{Name: "projects", Enabled: true, Force: true, ScopedRead: true, WriteCovered: true},
		},
		Definers: []pg.DefinerFn{{Signature: "list_org_ids()", OwnedBySchemaOwner: true}},
	}
}

func TestCheckRLS_ACleanReportPasses(t *testing.T) {
	warn, err := CheckRLS("writer", cleanReport(), RLSConfig{})
	require.NoError(t, err)
	assert.Empty(t, warn)
}

func TestCheckRLS_RefusesEveryReason(t *testing.T) {
	cases := map[string]struct {
		mutate func(*pg.RLSReport)
		want   string
	}{
		"superuser":      {func(r *pg.RLSReport) { r.Reach = []pg.RoleReach{{Role: "postgres", Super: true}} }, `role "postgres" is a superuser`},
		"bypassrls":      {func(r *pg.RLSReport) { r.Reach = []pg.RoleReach{{Role: "zzz_owner", BypassRLS: true}} }, `role "zzz_owner" has BYPASSRLS`},
		"schema owner":   {func(r *pg.RLSReport) { r.Reach = []pg.RoleReach{{Role: "oagent_owner", OwnsSchema: true}} }, `role "oagent_owner" owns the schema`},
		"table owner":    {func(r *pg.RLSReport) { r.Reach = []pg.RoleReach{{Role: "oagent_owner", OwnsTable: true}} }, `role "oagent_owner" owns tables`},
		"rls off":        {func(r *pg.RLSReport) { r.Tables[1].Enabled = false }, "table projects: ROW LEVEL SECURITY is not enabled"},
		"no force":       {func(r *pg.RLSReport) { r.Tables[1].Force = false }, "table projects: FORCE ROW LEVEL SECURITY is missing"},
		"no policy":      {func(r *pg.RLSReport) { r.Tables[1].ScopedRead = false }, "table projects: no policy reads the tenant scope"},
		"no write":       {func(r *pg.RLSReport) { r.Tables[1].WriteCovered = false }, "table projects: no policy covers writes"},
		"unscoped":       {func(r *pg.RLSReport) { r.Tables[1].Unscoped = true }, "table projects: a policy does not read the tenant scope"},
		"leaks":          {func(r *pg.RLSReport) { r.Tables[1].Leaks = true }, "table projects: a scope no tenant owns still sees rows"},
		"definer owner":  {func(r *pg.RLSReport) { r.Definers[0].OwnedBySchemaOwner = false }, "function list_org_ids(): not owned by the schema owner"},
		"definer public": {func(r *pg.RLSReport) { r.Definers[0].PublicExecute = true }, "function list_org_ids(): PUBLIC may execute it"},
		"row_security":   {func(r *pg.RLSReport) { r.RowSecurity = "off" }, "row_security is off"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := cleanReport()
			c.mutate(&r)
			_, err := CheckRLS("reader", r, RLSConfig{})
			require.True(t, IsRLSError(err), "got %v", err)
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, err.Error(), "reader")
		})
	}
}

func TestCheckRLS_AllowBypassRelaxesOnlyTheRoleCheck(t *testing.T) {
	r := cleanReport()
	r.Reach = []pg.RoleReach{{Role: "zzz_owner", BypassRLS: true, OwnsTable: true}}
	warn, err := CheckRLS("writer", r, RLSConfig{AllowBypass: true})
	require.NoError(t, err)
	assert.Contains(t, warn, `role "zzz_owner" has BYPASSRLS`)
	assert.Contains(t, warn, "db.rls.allowBypass")

	r.Tables[0].Force = false
	_, err = CheckRLS("writer", r, RLSConfig{AllowBypass: true})
	require.True(t, IsRLSError(err), "a table posture is never relaxed")
	assert.NotContains(t, err.Error(), "BYPASSRLS")

	r = cleanReport()
	r.RowSecurity = "off"
	_, err = CheckRLS("writer", r, RLSConfig{AllowBypass: true})
	require.True(t, IsRLSError(err), "row_security off is never relaxed")
}

func TestCheckRLS_ListsEveryReasonOnce(t *testing.T) {
	r := cleanReport()
	r.Tables[0].Force, r.Definers[0].PublicExecute = false, true
	_, err := CheckRLS("writer", r, RLSConfig{})
	e, ok := errors.AsType[*RLSError](err)
	require.True(t, ok)
	assert.Len(t, e.Reasons, 2)
}

func TestProbeLeaks_MarksATableAnUnownedScopeCanRead(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT set_config\('app\.current_org_id', \$1, true\)`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM "app"\."orgs"\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM "app"\."projects"\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()

	tables := []pg.TableRLS{{Name: "orgs"}, {Name: "projects"}}
	require.NoError(t, ProbeLeaks(context.Background(), pool.W, "app", tables))
	assert.False(t, tables[0].Leaks)
	assert.True(t, tables[1].Leaks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProbeLeaks_QueryErrorFailsTheProbe(t *testing.T) {
	pool, mock := mockPool(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT set_config`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(errors.New("gone"))
	mock.ExpectRollback()

	err := ProbeLeaks(context.Background(), pool.W, "app", []pg.TableRLS{{Name: "orgs"}})
	require.ErrorContains(t, err, "db: rls leak probe: orgs: gone")
	assert.NoError(t, mock.ExpectationsWereMet())
}
