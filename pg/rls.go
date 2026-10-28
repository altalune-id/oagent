package pg

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// RLSReport describes how a connection's roles and one schema's tables and functions stand against row-level security.
type RLSReport struct {
	SessionUser string
	CurrentUser string
	RowSecurity string
	Reach       []RoleReach
	Tables      []TableRLS
	Definers    []DefinerFn
}

// RoleReach is a role the session or current user is, or is a member of, whose powers defeat row-level security.
type RoleReach struct {
	Role       string
	Super      bool
	BypassRLS  bool
	OwnsSchema bool
	OwnsTable  bool
}

// TableRLS is the row-level-security posture of one tenant table.
type TableRLS struct {
	Name         string
	Enabled      bool
	Force        bool
	ScopedRead   bool
	WriteCovered bool
	Unscoped     bool
	Leaks        bool
}

// DefinerFn is one SECURITY DEFINER function in the schema.
type DefinerFn struct {
	Signature          string
	OwnedBySchemaOwner bool
	PublicExecute      bool
}

// ProbeRLS reads the RLS facts of db's roles and of schema's tenant tables, policies (judged by scopeGUC) and definer functions.
func ProbeRLS(ctx context.Context, db *sql.DB, schema, tenantColumn, scopeGUC string, extraTables ...string) (RLSReport, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return RLSReport{}, fmt.Errorf("pg: rls probe: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var r RLSReport
	if err := conn.QueryRowContext(ctx, `SELECT session_user, current_user, current_setting('row_security')`).
		Scan(&r.SessionUser, &r.CurrentUser, &r.RowSecurity); err != nil {
		return RLSReport{}, fmt.Errorf("pg: rls probe: who: %w", err)
	}
	if r.Reach, err = loadReach(ctx, conn, schema); err != nil {
		return RLSReport{}, err
	}
	if r.Tables, err = loadTables(ctx, conn, schema, tenantColumn, scopeGUC, extraTables); err != nil {
		return RLSReport{}, err
	}
	if r.Definers, err = loadDefiners(ctx, conn, schema); err != nil {
		return RLSReport{}, err
	}
	return r, nil
}

func loadReach(ctx context.Context, conn *sql.Conn, schema string) ([]RoleReach, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT r.rolname, r.rolsuper, r.rolbypassrls,
		       EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname = $1 AND n.nspowner = r.oid),
		       EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		               WHERE n.nspname = $1 AND c.relkind IN ('r', 'p') AND c.relowner = r.oid)
		FROM pg_roles r
		WHERE pg_has_role(session_user, r.oid, 'MEMBER') OR pg_has_role(current_user, r.oid, 'MEMBER')
		ORDER BY r.rolname`, schema)
	if err != nil {
		return nil, fmt.Errorf("pg: rls probe: roles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RoleReach
	for rows.Next() {
		var rr RoleReach
		if err := rows.Scan(&rr.Role, &rr.Super, &rr.BypassRLS, &rr.OwnsSchema, &rr.OwnsTable); err != nil {
			return nil, fmt.Errorf("pg: rls probe: scan roles: %w", err)
		}
		if rr.Super || rr.BypassRLS || rr.OwnsSchema || rr.OwnsTable {
			out = append(out, rr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg: rls probe: roles: %w", err)
	}
	return out, nil
}

func loadTables(ctx context.Context, conn *sql.Conn, schema, tenantColumn, scopeGUC string, extra []string) ([]TableRLS, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind IN ('r', 'p')
		  AND (c.relname = ANY ($3)
		       OR EXISTS (SELECT 1 FROM pg_attribute a
		                  WHERE a.attrelid = c.oid AND a.attname = $2 AND a.attnum > 0 AND NOT a.attisdropped))
		ORDER BY c.relname`, schema, tenantColumn, extra)
	if err != nil {
		return nil, fmt.Errorf("pg: rls probe: tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TableRLS
	for rows.Next() {
		var tr TableRLS
		if err := rows.Scan(&tr.Name, &tr.Enabled, &tr.Force); err != nil {
			return nil, fmt.Errorf("pg: rls probe: scan tables: %w", err)
		}
		out = append(out, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg: rls probe: tables: %w", err)
	}
	markers, err := scopeMarkers(ctx, conn, schema, scopeGUC)
	if err != nil {
		return nil, err
	}
	posture, err := loadPolicyPosture(ctx, conn, schema, markers)
	if err != nil {
		return nil, err
	}
	for i := range out {
		p := posture[out[i].Name]
		out[i].ScopedRead, out[i].Unscoped = p.scopedRead, p.unscoped
		out[i].WriteCovered = p.scopedRead && p.writeCovered
	}
	return out, nil
}

type policyPosture struct {
	scopedRead   bool
	unscoped     bool
	writeCovered bool
}

// SECURITY: an empty with_check reuses qual for writes; a non-empty one needs its own scope marker or writes reach any tenant.
func (p *policyPosture) observe(qual, withCheck, cmd string, markers []string) {
	qualScoped := qual != "" && containsMarker(qual, markers)
	checkScoped := withCheck != "" && containsMarker(withCheck, markers)
	if qualScoped {
		p.scopedRead = true
	}
	if (qual != "" && !qualScoped) || (withCheck != "" && !checkScoped) || (qual == "" && withCheck == "") {
		p.unscoped = true
	}
	switch strings.ToUpper(strings.TrimSpace(cmd)) {
	case "ALL", "INSERT", "UPDATE", "DELETE":
		p.writeCovered = true
	}
}

func containsMarker(expr string, markers []string) bool {
	return slices.ContainsFunc(markers, func(m string) bool { return strings.Contains(expr, m) })
}

func loadPolicyPosture(ctx context.Context, conn *sql.Conn, schema string, markers []string) (map[string]policyPosture, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT tablename, COALESCE(qual, ''), COALESCE(with_check, ''), cmd
		FROM pg_policies WHERE schemaname = $1`, schema)
	if err != nil {
		return nil, fmt.Errorf("pg: rls probe: policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]policyPosture{}
	for rows.Next() {
		var table, qual, withCheck, cmd string
		if err := rows.Scan(&table, &qual, &withCheck, &cmd); err != nil {
			return nil, fmt.Errorf("pg: rls probe: scan policies: %w", err)
		}
		p := out[table]
		p.observe(qual, withCheck, cmd, markers)
		out[table] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg: rls probe: policies: %w", err)
	}
	return out, nil
}

// NOTE: policies read the GUC through NULL-safe helper functions, so a policy's qual names the helper, not the GUC.
func scopeMarkers(ctx context.Context, conn *sql.Conn, schema, scopeGUC string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT p.proname || '('
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = $1 AND strpos(p.prosrc, $2) > 0`, schema, scopeGUC)
	if err != nil {
		return nil, fmt.Errorf("pg: rls probe: helpers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	markers := []string{scopeGUC}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("pg: rls probe: scan helpers: %w", err)
		}
		markers = append(markers, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg: rls probe: helpers: %w", err)
	}
	return markers, nil
}

func loadDefiners(ctx context.Context, conn *sql.Conn, schema string) ([]DefinerFn, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT p.oid::regprocedure::text, p.proowner = n.nspowner, has_function_privilege('public', p.oid, 'EXECUTE')
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = $1 AND p.prosecdef`, schema)
	if err != nil {
		return nil, fmt.Errorf("pg: rls probe: definers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DefinerFn
	for rows.Next() {
		var d DefinerFn
		if err := rows.Scan(&d.Signature, &d.OwnedBySchemaOwner, &d.PublicExecute); err != nil {
			return nil, fmt.Errorf("pg: rls probe: scan definers: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg: rls probe: definers: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Signature < out[j].Signature })
	return out, nil
}
