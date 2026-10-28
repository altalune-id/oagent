//go:build integration

package schema_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

type lookup struct {
	call    string
	columns []string
}

func lookups() map[string]lookup {
	orgColumns := []string{"id", "public_id", "slug", "name", "created_by", "created_at", "updated_at", "is_system"}
	return map[string]lookup{
		"list_org_ids()":                        {"list_org_ids()", []string{"id", "created_at"}},
		"resolve_system_org()":                  {"resolve_system_org()", orgColumns},
		"resolve_org_by_slug(text)":             {"resolve_org_by_slug('x')", orgColumns},
		"resolve_org_by_public_id(text)":        {"resolve_org_by_public_id('x')", orgColumns},
		"resolve_invite_by_token_hash(text)":    {"resolve_invite_by_token_hash('x')", []string{"id", "public_id", "org_id", "email", "role", "draft", "invited_by", "expires_at", "accepted_at", "accepted_by", "revoked_at", "created_at", "updated_at"}},
		"resolve_project_by_public_id(text)":    {"resolve_project_by_public_id('x')", []string{"id", "org_id"}},
		"resolve_agent_by_public_id(text)":      {"resolve_agent_by_public_id('x')", []string{"id", "org_id", "project_id"}},
		"resolve_invite_by_public_id(text)":     {"resolve_invite_by_public_id('x')", []string{"id", "public_id", "org_id", "email", "role", "draft", "invited_by", "expires_at", "accepted_at", "accepted_by", "revoked_at", "created_at", "updated_at"}},
		"resolve_api_key_by_secret_hash(bytea)": {"resolve_api_key_by_secret_hash('\\x00'::bytea)", []string{"id", "org_id", "project_id", "kind", "all_projects", "project_ids", "name", "secret_hint", "scopes", "resource_ids", "created_by", "created_at", "expires_at", "revoked_at", "last_used_at"}},
	}
}

// NOTE: secret_hint is the four-character hint the console shows, not part of the secret.
func isSecretColumn(name string) bool {
	if name == "secret_hint" {
		return false
	}
	return strings.HasSuffix(name, "_hash") || strings.HasSuffix(name, "_enc") || strings.HasPrefix(name, "sealed_") || strings.HasPrefix(name, "secret")
}

// SECURITY: a definer runs as the owner and skips row level security, so it must return public columns only, run with a pinned search_path and be closed to PUBLIC.
func TestLookups_ArePublicColumnDefinersClosedToPublic(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	ctx := t.Context()
	for sig, l := range lookups() {
		t.Run(sig, func(t *testing.T) {
			rows, err := sqlDB.QueryContext(ctx, "SELECT * FROM "+schemaName+"."+l.call+" LIMIT 0")
			require.NoError(t, err)
			cols, err := rows.Columns()
			require.NoError(t, err)
			require.NoError(t, rows.Close())
			assert.Equal(t, l.columns, cols)
			for _, c := range cols {
				assert.False(t, isSecretColumn(c), "%s returns the secret column %s", sig, c)
			}

			fn := schemaName + "." + sig
			var definer, publicExec bool
			var config string
			require.NoError(t, sqlDB.QueryRowContext(ctx,
				`SELECT p.prosecdef, array_to_string(coalesce(p.proconfig, '{}'), '|'), has_function_privilege('public', $1::regprocedure, 'EXECUTE')
				 FROM pg_proc p WHERE p.oid = $1::regprocedure`, fn).Scan(&definer, &config, &publicExec))
			assert.True(t, definer, "SECURITY DEFINER")
			assert.False(t, publicExec, "PUBLIC must not execute %s", sig)
			assert.Equal(t, "search_path="+schemaName+", pg_catalog, pg_temp", config)
		})
	}
}

func TestLookups_ResolveTheMatchingRowOnly(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	ctx := t.Context()
	s := schemaName + "."
	now := time.Now().UTC()
	user, sys, other := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO `+s+`users (id, public_id, email, created_at, updated_at) VALUES ($1, 'u1', 'u@example.com', $2, $2)`, user, now)
	exec(`INSERT INTO `+s+`orgs (id, public_id, slug, name, created_at, updated_at, is_system) VALUES ($1, 'osys', 'sys', 'Sys', $3, $3, true), ($2, 'oother', 'other', 'Other', $3, $3, false)`, sys, other, now)
	exec(`INSERT INTO `+s+`memberships (id, public_id, org_id, user_id, role, created_at) VALUES ($1, 'm1', $2, $3, 'owner', $4)`, uuid.New(), other, user, now)
	project := uuid.New()
	exec(`INSERT INTO `+s+`projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, 'p1', $2, 'web', 'Web', $3, $3)`, project, other, now)
	agent := uuid.New()
	exec(`INSERT INTO `+s+`agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, 'a1', $2, $3, 'bot', 'Bot', $4, $4)`, agent, other, project, now)
	invite := uuid.New()
	exec(`INSERT INTO `+s+`invites (id, public_id, org_id, email, role, token_hash, expires_at, created_at, updated_at, email_only) VALUES ($1, 'i1', $2, 'a@example.com', 'member', 'tok', $3, $3, $3, false)`, invite, other, now.Add(time.Hour))
	exec(`INSERT INTO `+s+`api_keys (id, org_id, kind, all_projects, name, secret_hash, secret_hint, created_at) VALUES ($1, $2, 'org', true, 'k', '\x01'::bytea, 'abcd', $3)`, uuid.New(), other, now)

	ids := func(q string, args ...any) []uuid.UUID {
		t.Helper()
		rows, err := sqlDB.QueryContext(ctx, q, args...)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var out []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			cols, err := rows.Columns()
			require.NoError(t, err)
			dest := make([]any, len(cols))
			for i := range dest {
				dest[i] = new(any)
			}
			dest[0] = &id
			require.NoError(t, rows.Scan(dest...))
			out = append(out, id)
		}
		require.NoError(t, rows.Err())
		return out
	}
	assert.ElementsMatch(t, []uuid.UUID{sys, other}, ids(`SELECT * FROM `+s+`list_org_ids()`))
	assert.Equal(t, []uuid.UUID{sys}, ids(`SELECT * FROM `+s+`resolve_system_org()`))
	assert.Equal(t, []uuid.UUID{other}, ids(`SELECT * FROM `+s+`resolve_org_by_slug($1)`, "other"))
	assert.Equal(t, []uuid.UUID{other}, ids(`SELECT * FROM `+s+`resolve_org_by_public_id($1)`, "oother"))
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_org_by_slug($1)`, "nope"))

	var inviteOrg, keyOrg uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT org_id FROM `+s+`resolve_invite_by_token_hash($1)`, "tok").Scan(&inviteOrg))
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT org_id FROM `+s+`resolve_api_key_by_secret_hash($1)`, []byte{1}).Scan(&keyOrg))
	assert.Equal(t, other, inviteOrg)
	assert.Equal(t, other, keyOrg)
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_invite_by_token_hash($1)`, "nope"))
	assert.Equal(t, []uuid.UUID{invite}, ids(`SELECT * FROM `+s+`resolve_invite_by_public_id($1)`, "i1"))
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_invite_by_public_id($1)`, "nope"))
	assert.Equal(t, []uuid.UUID{project}, ids(`SELECT * FROM `+s+`resolve_project_by_public_id($1)`, "p1"))
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_project_by_public_id($1)`, "nope"))
	assert.Equal(t, []uuid.UUID{agent}, ids(`SELECT * FROM `+s+`resolve_agent_by_public_id($1)`, "a1"))
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_agent_by_public_id($1)`, "nope"))
	assert.Empty(t, ids(`SELECT * FROM `+s+`resolve_api_key_by_secret_hash($1)`, []byte{2}))
}

func TestLookups_TheServiceRoleMayExecuteThem(t *testing.T) {
	svc, _, schemaName := pgtest.MigratedRLS(t)
	for sig := range lookups() {
		var ok bool
		require.NoError(t, svc.W.QueryRowContext(t.Context(),
			`SELECT has_function_privilege(current_user, $1::regprocedure, 'EXECUTE')`, schemaName+"."+sig).Scan(&ok))
		assert.True(t, ok, sig)
	}
}

// SECURITY: every definer in the schema is one this test checks; a new one must be added to lookups.
func TestLookups_AreTheOnlyDefiners(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	rows, err := sqlDB.QueryContext(t.Context(),
		`SELECT p.oid::regprocedure::text FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		 WHERE n.nspname = $1 AND p.prosecdef`, schemaName)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var sig string
		require.NoError(t, rows.Scan(&sig))
		got = append(got, strings.TrimPrefix(sig, schemaName+"."))
	}
	require.NoError(t, rows.Err())
	want := make([]string, 0, len(lookups()))
	for sig := range lookups() {
		want = append(want, sig)
	}
	assert.ElementsMatch(t, want, got)
}

func TestLookups_WorkForTheServiceRoleWithNoScope(t *testing.T) {
	svc, admin, schemaName := pgtest.MigratedRLS(t)
	now := time.Now().UTC()
	org := uuid.New()
	_, err := admin.ExecContext(t.Context(), `INSERT INTO `+schemaName+`.orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, 'o1', 'acme', 'Acme', $2, $2)`, org, now)
	require.NoError(t, err)
	var got uuid.UUID
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `SELECT id FROM `+schemaName+`.resolve_org_by_slug('acme')`).Scan(&got))
	assert.Equal(t, org, got)
	var n int
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `SELECT count(*) FROM `+schemaName+`.orgs`).Scan(&n))
	assert.Zero(t, n, "the table itself stays hidden without a scope")
}
