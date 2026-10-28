//go:build integration

package pg_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/pg"
)

const rlsGUC = "app.current_org_id"

type rlsFixture struct {
	t      *testing.T
	dsn    string
	admin  *sql.DB
	schema string
	owner  string
	svc    string
}

func newRLSFixture(t *testing.T) *rlsFixture {
	t.Helper()
	h := pgtest.New(t)
	admin := h.OpenDB(t)
	f := &rlsFixture{t: t, dsn: h.DSN, admin: admin, schema: h.Schema}
	f.owner = f.role("NOLOGIN")
	f.svc = f.role("LOGIN NOBYPASSRLS PASSWORD 'svc-password'")
	s, o := f.q(f.schema), f.q(f.owner)
	f.exec(
		"ALTER SCHEMA "+s+" OWNER TO "+o,
		"SET ROLE "+o,
		"CREATE FUNCTION "+s+".current_org_id() RETURNS uuid LANGUAGE sql STABLE AS $$ SELECT NULLIF(current_setting('"+rlsGUC+"', true), '')::uuid $$",
		"CREATE TABLE "+s+".orgs (id uuid PRIMARY KEY)",
		"CREATE TABLE "+s+".things (id uuid PRIMARY KEY, org_id uuid NOT NULL)",
		"CREATE TABLE "+s+".globals (id uuid PRIMARY KEY)",
		"CREATE FUNCTION "+s+".lookup() RETURNS SETOF uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog AS $$ SELECT id FROM "+s+".orgs $$",
		"REVOKE ALL ON FUNCTION "+s+".lookup() FROM PUBLIC",
		"RESET ROLE",
	)
	for _, table := range []string{"orgs", "things"} {
		col := "org_id"
		if table == "orgs" {
			col = "id"
		}
		f.exec(
			"ALTER TABLE "+s+"."+table+" ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE "+s+"."+table+" FORCE ROW LEVEL SECURITY",
			"CREATE POLICY tenant ON "+s+"."+table+" FOR ALL USING ("+col+" = "+s+".current_org_id()) WITH CHECK ("+col+" = "+s+".current_org_id())",
		)
	}
	f.exec(
		"GRANT USAGE ON SCHEMA "+s+" TO "+f.q(f.svc),
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA "+s+" TO "+f.q(f.svc),
		"GRANT EXECUTE ON FUNCTION "+s+".lookup() TO "+f.q(f.svc),
	)
	return f
}

func (f *rlsFixture) q(name string) string { return pgx.Identifier{name}.Sanitize() }

func (f *rlsFixture) exec(stmts ...string) {
	f.t.Helper()
	for _, stmt := range stmts {
		_, err := f.admin.ExecContext(f.t.Context(), stmt)
		require.NoError(f.t, err, stmt)
	}
}

func (f *rlsFixture) role(attrs string) string {
	f.t.Helper()
	id, err := nanoid.New(10)
	require.NoError(f.t, err)
	name := "pgt_rls_" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(id))
	if _, err := f.admin.ExecContext(f.t.Context(), "CREATE ROLE "+f.q(name)+" "+attrs); err != nil {
		f.t.Skipf("cannot create roles on this server: %v", err)
	}
	f.t.Cleanup(func() {
		for _, stmt := range []string{"DROP OWNED BY " + f.q(name) + " CASCADE", "DROP ROLE IF EXISTS " + f.q(name)} {
			if _, err := f.admin.ExecContext(context.Background(), stmt); err != nil {
				f.t.Errorf("%s: %v", stmt, err)
			}
		}
	})
	return name
}

func (f *rlsFixture) login(role, password string) *sql.DB {
	f.t.Helper()
	cfg, err := pgx.ParseConfig(f.dsn)
	require.NoError(f.t, err)
	cfg.User, cfg.Password = role, password
	sqlDB := stdlib.OpenDB(*cfg)
	f.t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func (f *rlsFixture) probe(conn *sql.DB) pg.RLSReport {
	f.t.Helper()
	r, err := pg.ProbeRLS(f.t.Context(), conn, f.schema, "org_id", rlsGUC, "orgs")
	require.NoError(f.t, err)
	return r
}

func (f *rlsFixture) svcDB() *sql.DB { return f.login(f.svc, "svc-password") }

func reachOf(r pg.RLSReport, role string) (pg.RoleReach, bool) {
	for _, rr := range r.Reach {
		if rr.Role == role {
			return rr, true
		}
	}
	return pg.RoleReach{}, false
}

func tableOf(t *testing.T, r pg.RLSReport, name string) pg.TableRLS {
	t.Helper()
	for _, tr := range r.Tables {
		if tr.Name == name {
			return tr
		}
	}
	t.Fatalf("table %s not in report: %+v", name, r.Tables)
	return pg.TableRLS{}
}

func TestProbeRLS_ACleanServiceRoleReachesNothing(t *testing.T) {
	f := newRLSFixture(t)
	r := f.probe(f.svcDB())

	assert.Equal(t, f.svc, r.SessionUser)
	assert.Equal(t, f.svc, r.CurrentUser)
	assert.Equal(t, "on", r.RowSecurity)
	assert.Empty(t, r.Reach)
	require.Len(t, r.Tables, 2, "tables carrying org_id plus the named extras, never the global table")
	for _, name := range []string{"orgs", "things"} {
		assert.Equal(t, pg.TableRLS{Name: name, Enabled: true, Force: true, ScopedRead: true, WriteCovered: true}, tableOf(t, r, name))
	}
	require.Len(t, r.Definers, 1)
	assert.Equal(t, pg.DefinerFn{Signature: f.schema + ".lookup()", OwnedBySchemaOwner: true}, r.Definers[0])
}

func TestProbeRLS_Superuser(t *testing.T) {
	f := newRLSFixture(t)
	r := f.probe(f.admin)
	rr, ok := reachOf(r, r.SessionUser)
	require.True(t, ok)
	assert.True(t, rr.Super)
}

func TestProbeRLS_BypassRLS(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("ALTER ROLE " + f.q(f.svc) + " BYPASSRLS")
	rr, ok := reachOf(f.probe(f.svcDB()), f.svc)
	require.True(t, ok)
	assert.True(t, rr.BypassRLS)
}

func TestProbeRLS_MemberOfABypassRole(t *testing.T) {
	for name, option := range map[string]string{"inherit only": "INHERIT TRUE, SET FALSE", "set only": "INHERIT FALSE, SET TRUE"} {
		t.Run(name, func(t *testing.T) {
			f := newRLSFixture(t)
			bypass := f.role("NOLOGIN BYPASSRLS")
			f.exec(fmt.Sprintf("GRANT %s TO %s WITH %s", f.q(bypass), f.q(f.svc), option))
			rr, ok := reachOf(f.probe(f.svcDB()), bypass)
			require.True(t, ok, "membership reaches the bypass role on any path")
			assert.True(t, rr.BypassRLS)
		})
	}
}

func TestProbeRLS_MemberOfTheOwner(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("GRANT " + f.q(f.owner) + " TO " + f.q(f.svc))
	rr, ok := reachOf(f.probe(f.svcDB()), f.owner)
	require.True(t, ok)
	assert.True(t, rr.OwnsSchema)
	assert.True(t, rr.OwnsTable)
}

func TestProbeRLS_NoForce(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("ALTER TABLE " + f.q(f.schema) + ".things NO FORCE ROW LEVEL SECURITY")
	tr := tableOf(t, f.probe(f.svcDB()), "things")
	assert.True(t, tr.Enabled)
	assert.False(t, tr.Force)
}

func TestProbeRLS_MissingPolicy(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("DROP POLICY tenant ON " + f.q(f.schema) + ".things")
	tr := tableOf(t, f.probe(f.svcDB()), "things")
	assert.False(t, tr.ScopedRead)
	assert.False(t, tr.WriteCovered)
}

func TestProbeRLS_UnscopedOrReadOnlyPolicy(t *testing.T) {
	f := newRLSFixture(t)
	s := f.q(f.schema)
	f.exec(
		"DROP POLICY tenant ON "+s+".things",
		"CREATE POLICY open_read ON "+s+".things FOR SELECT USING (true)",
		"DROP POLICY tenant ON "+s+".orgs",
		"CREATE POLICY read_only ON "+s+".orgs FOR SELECT USING (id = "+s+".current_org_id())",
	)
	r := f.probe(f.svcDB())
	assert.True(t, tableOf(t, r, "things").Unscoped, "USING (true) is an unscoped policy")
	orgs := tableOf(t, r, "orgs")
	assert.True(t, orgs.ScopedRead)
	assert.False(t, orgs.WriteCovered, "a SELECT-only policy leaves writes uncovered")
}

func TestProbeRLS_PublicExecutableDefiner(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("GRANT EXECUTE ON FUNCTION " + f.q(f.schema) + ".lookup() TO PUBLIC")
	r := f.probe(f.svcDB())
	require.Len(t, r.Definers, 1)
	assert.True(t, r.Definers[0].PublicExecute)
}

func TestProbeRLS_DefinerOwnedByAnotherRole(t *testing.T) {
	f := newRLSFixture(t)
	other := f.role("NOLOGIN")
	f.exec("ALTER FUNCTION " + f.q(f.schema) + ".lookup() OWNER TO " + f.q(other))
	r := f.probe(f.svcDB())
	require.Len(t, r.Definers, 1)
	assert.False(t, r.Definers[0].OwnedBySchemaOwner)
}

func TestProbeRLS_RowSecurityOff(t *testing.T) {
	f := newRLSFixture(t)
	f.exec("ALTER ROLE " + f.q(f.svc) + " SET row_security = off")
	assert.Equal(t, "off", f.probe(f.svcDB()).RowSecurity)
}
