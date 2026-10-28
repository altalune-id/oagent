//go:build integration

package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

const (
	// RLSOwnerRole owns every migrated object in MigratedRLS schemas, like oagent_owner in production.
	RLSOwnerRole = "oagent_test_owner"
	// RLSServiceRole is the NOBYPASSRLS login MigratedRLS pools use, like oagent_service in production.
	RLSServiceRole      = "oagent_test_service"
	rlsServicePassword  = "oagent-test"
	rlsRolesAdvisoryKey = 7_301_955_117
)

//nolint:gochecknoglobals // roles are cluster-wide: provision them once per server per test binary.
var rlsRoles struct {
	mu   sync.Mutex
	done map[string]error
}

// MigratedRLS migrates a fresh schema as RLSOwnerRole and returns an RLSServiceRole pool, an admin handle and the schema.
func MigratedRLS(t *testing.T) (svc db.Pool, admin *sql.DB, schemaName string) {
	t.Helper()
	h := New(t)
	admin = h.OpenDB(t)
	if err := rlsRolesOnce(t.Context(), h.DSN, admin); err != nil {
		t.Fatalf("pgtest: cannot provision the RLS test roles; %s must connect as a role with CREATEROLE and BYPASSRLS: %v", envDSN, err)
	}

	quoted := pgx.Identifier{h.Schema}.Sanitize()
	exec(t, admin, "ALTER SCHEMA "+quoted+" OWNER TO "+RLSOwnerRole)

	cfg := migrator.Config{Schema: h.Schema, Role: RLSOwnerRole}
	migDB, err := migrator.Open(t.Context(), h.DSN, cfg)
	if err != nil {
		t.Fatalf("pgtest: open migrator: %v", err)
	}
	err = schema.MigrateUp(t.Context(), migDB, cfg)
	_ = migDB.Close()
	if err != nil {
		t.Fatalf("pgtest: migrate as %s: %v", RLSOwnerRole, err)
	}

	grantService(t, admin, quoted)
	svcDB := openAs(t, h, RLSServiceRole, rlsServicePassword)
	return db.Pool{W: svcDB, R: svcDB}, admin, h.Schema
}

func grantService(t *testing.T, admin *sql.DB, quotedSchema string) {
	t.Helper()
	for _, stmt := range []string{
		"REVOKE CREATE ON SCHEMA " + quotedSchema + " FROM PUBLIC",
		"GRANT USAGE ON SCHEMA " + quotedSchema + " TO " + RLSServiceRole,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + quotedSchema + " TO " + RLSServiceRole,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA " + quotedSchema + " TO " + RLSServiceRole,
		"REVOKE ALL ON " + quotedSchema + ".goose_db_version FROM " + RLSServiceRole,
		"GRANT SELECT ON " + quotedSchema + ".goose_db_version TO " + RLSServiceRole,
		"REVOKE ALL ON ALL FUNCTIONS IN SCHEMA " + quotedSchema + " FROM PUBLIC",
		"GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA " + quotedSchema + " TO " + RLSServiceRole,
	} {
		exec(t, admin, stmt)
	}
}

func openAs(t *testing.T, h *Handle, user, password string) *sql.DB {
	t.Helper()
	connCfg, err := pgx.ParseConfig(h.DSN)
	if err != nil {
		t.Fatalf("pgtest: parse DSN: %v", err)
	}
	connCfg.User = user
	connCfg.Password = password
	connCfg.RuntimeParams["search_path"] = h.Schema
	sqlDB := stdlib.OpenDB(*connCfg)
	if err := sqlDB.PingContext(t.Context()); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("pgtest: connect as %s: %v", user, err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func rlsRolesOnce(ctx context.Context, dsn string, admin *sql.DB) error {
	rlsRoles.mu.Lock()
	defer rlsRoles.mu.Unlock()
	if err, ok := rlsRoles.done[dsn]; ok {
		return err
	}
	err := ensureRLSRoles(ctx, admin, RLSOwnerRole, RLSServiceRole)
	if rlsRoles.done == nil {
		rlsRoles.done = map[string]error{}
	}
	rlsRoles.done[dsn] = err
	return err
}

// SECURITY: the service role is reset to NOBYPASSRLS with no memberships on every run, or the RLS tests built on it would pass vacuously.
func ensureRLSRoles(ctx context.Context, admin *sql.DB, owner, service string) error {
	tx, err := admin.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(rlsRolesAdvisoryKey)); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	ownerID, serviceID := pgx.Identifier{owner}.Sanitize(), pgx.Identifier{service}.Sanitize()
	stmts := []string{
		upsertRole(owner, ownerID, "NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE BYPASSRLS"),
		upsertRole(service, serviceID, "LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '"+rlsServicePassword+"'"),
		fmt.Sprintf(`DO $$ DECLARE g text; BEGIN
			FOR g IN SELECT r.rolname FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.roleid
			          WHERE m.member = (SELECT oid FROM pg_roles WHERE rolname = %s) LOOP
				EXECUTE format('REVOKE %%I FROM %%I', g, %s);
			END LOOP; END $$`, quoteLiteral(service), quoteLiteral(service)),
		"DO $$ BEGIN EXECUTE format('GRANT CONNECT ON DATABASE %I TO " + serviceID + "', current_database()); END $$",
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return tx.Commit()
}

func upsertRole(name, quoted, attrs string) string {
	return fmt.Sprintf(`DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN
			CREATE ROLE %s %s;
		ELSE
			ALTER ROLE %s %s;
		END IF;
	END $$`, quoteLiteral(name), quoted, attrs, quoted, attrs)
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func exec(t *testing.T, sqlDB *sql.DB, stmt string) {
	t.Helper()
	if _, err := sqlDB.ExecContext(t.Context(), stmt); err != nil {
		t.Fatalf("pgtest: %s: %v", stmt, err)
	}
}
