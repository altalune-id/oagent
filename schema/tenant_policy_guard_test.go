package schema

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var (
	createTableRe = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+\{\{\.Schema\}\}\.\{\{\.TablePrefix\}\}(\w+)\s*\((.*?)\n\);`)
	orgIDColumnRe = regexp.MustCompile(`(?m)^\s*org_id\s+UUID`)
	policyRe      = regexp.MustCompile(`(?s)CREATE POLICY \{\{\.TablePrefix\}\}(\w+) ON .*?;`)
)

//nolint:gochecknoglobals // the tables a user-only scope may read through its memberships.
var userReadable = map[string]bool{"orgs": true, "memberships": true}

// SECURITY: an inlined current_setting(...)::uuid throws 22P02 once a pooled connection has served a tenant, because a transaction-local set_config resets to empty string rather than NULL.
func TestMigrations_PoliciesUseTheGuardedScopeHelpers(t *testing.T) {
	sources := migrationSources(t)
	for _, guc := range []string{"app.current_org_id", "app.current_user_id"} {
		helperDef := "SELECT NULLIF(current_setting('" + guc + "', true), '')::uuid"
		defs := 0
		for name, src := range sources {
			defs += strings.Count(src, helperDef)
			if inlined := strings.Count(src, "current_setting('"+guc+"'") - strings.Count(src, helperDef); inlined > 0 {
				t.Errorf("%s inlines current_setting('%s') %d time(s); call its helper instead", name, guc, inlined)
			}
		}
		if defs != 1 {
			t.Errorf("the %s helper is defined %d times, want exactly 1", guc, defs)
		}
	}
}

// SECURITY: every org-owned table forces row level security and carries a policy that checks writes too, so a query that forgets its tenant filter returns nothing.
func TestMigrations_EveryOrgOwnedTableForcesItsPolicy(t *testing.T) {
	all := strings.Join(mapValues(migrationSources(t)), "\n")
	tables := 0
	for _, m := range createTableRe.FindAllStringSubmatch(all, -1) {
		table, body := m[1], m[2]
		if !orgIDColumnRe.MatchString(body) && table != "orgs" {
			continue
		}
		tables++
		target := `{{.Schema}}.{{.TablePrefix}}` + table
		for _, want := range []string{
			"ALTER TABLE " + target + " ENABLE ROW LEVEL SECURITY;",
			"ALTER TABLE " + target + " FORCE ROW LEVEL SECURITY;",
		} {
			if !strings.Contains(all, want) {
				t.Errorf("%s: missing %q", table, want)
			}
		}
		if userReadable[table] {
			checkSplitPolicies(t, all, table, target)
			continue
		}
		policy := regexp.MustCompile(`(?s)CREATE POLICY \{\{\.TablePrefix\}\}` + table + `_tenant ON ` + regexp.QuoteMeta(target) + `\s+FOR ALL\s+USING \(.*?\)\s+WITH CHECK \(.*?current_org_id\(\).*?\);`)
		if !policy.MatchString(all) {
			t.Errorf("%s: missing a FOR ALL policy %s_tenant with USING and a WITH CHECK on current_org_id()", table, table)
		}
	}
	if tables != 10 {
		t.Fatalf("found %d org-owned tables, want 10; add the new table to the tenant list in schema/README.md and to this guard", tables)
	}
}

// SECURITY: a user-only scope may read its own orgs and memberships but never write them; every write policy checks the org alone.
func checkSplitPolicies(t *testing.T, all, table, target string) {
	t.Helper()
	org := regexp.QuoteMeta(`{{.Schema}}.{{.TablePrefix}}current_org_id()`)
	key := "org_id"
	if table == "orgs" {
		key = "id"
	}
	scope := key + ` = ` + org
	q := regexp.QuoteMeta(target)
	for name, re := range map[string]*regexp.Regexp{
		"read":   regexp.MustCompile(`(?s)CREATE POLICY \{\{\.TablePrefix\}\}` + table + `_read ON ` + q + `\s+FOR SELECT\s+USING \(` + scope + ` OR .*?current_user_id\(\).*?\);`),
		"insert": regexp.MustCompile(`CREATE POLICY \{\{\.TablePrefix\}\}` + table + `_insert ON ` + q + `\s+FOR INSERT\s+WITH CHECK \(` + scope + `\);`),
		"update": regexp.MustCompile(`CREATE POLICY \{\{\.TablePrefix\}\}` + table + `_update ON ` + q + `\s+FOR UPDATE\s+USING \(` + scope + `\)\s+WITH CHECK \(` + scope + `\);`),
		"delete": regexp.MustCompile(`CREATE POLICY \{\{\.TablePrefix\}\}` + table + `_delete ON ` + q + `\s+FOR DELETE\s+USING \(` + scope + `\);`),
	} {
		if !re.MatchString(all) {
			t.Errorf("%s: missing the %s_%s policy that checks only %s", table, table, name, scope)
		}
	}
	if strings.Contains(all, "CREATE POLICY {{.TablePrefix}}"+table+"_tenant") {
		t.Errorf("%s: a FOR ALL policy would let a user-only scope write in any of its orgs", table)
	}
}

// SECURITY: the user branch appears only in a FOR SELECT policy, so a user-only scope can never write.
func TestMigrations_OnlyReadPoliciesUseTheUserScope(t *testing.T) {
	all := strings.Join(mapValues(migrationSources(t)), "\n")
	for _, m := range policyRe.FindAllStringSubmatch(all, -1) {
		if !strings.Contains(m[0], "current_user_id()") {
			continue
		}
		if !strings.Contains(m[0], "FOR SELECT") || !userReadable[strings.TrimSuffix(m[1], "_read")] {
			t.Errorf("policy %s reads current_user_id() outside a FOR SELECT policy on orgs or memberships", m[1])
		}
	}
}

func migrationSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(MigrationsFS(), ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(MigrationsFS(), e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(body)
	}
	if len(out) == 0 {
		t.Fatal("no migrations found; the guard would pass vacuously")
	}
	return out
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
