//go:build integration

package native_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	"altalune.id/oagent/internal/memory/native"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
)

type backend struct {
	pool   db.Pool
	admin  *sql.DB
	schema string
	seeded int
}

func newBackend(t *testing.T) *backend {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &backend{pool: pool, admin: admin, schema: schemaName}
}

func (b *backend) Facts() memory.Facts { return native.New(b.pool, b.schema, "") }

func (b *backend) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := b.admin.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func (b *backend) row(t *testing.T, table, cols, vals string, args ...any) uuid.UUID {
	t.Helper()
	id := uuid.New()
	b.exec(t, `INSERT INTO `+table+` (id, public_id, `+cols+`) VALUES ($1, $2, `+vals+`)`, append([]any{id, table[:1] + id.String()[:20]}, args...)...)
	return id
}

func (b *backend) Matrix(t *testing.T) []memorytest.Cell {
	t.Helper()
	var out []memorytest.Cell
	for range 2 {
		org := b.row(t, "orgs", "slug, name, created_at, updated_at", "$2, 'Org', now(), now()")
		ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org})
		for range 2 {
			prj := b.row(t, "projects", "org_id, slug, name, created_at, updated_at", "$3, $2, 'P', now(), now()", org)
			var agents, users, stores []uuid.UUID
			for range 2 {
				agents = append(agents, b.row(t, "agents", "org_id, project_id, slug, name, created_at, updated_at", "$3, $4, $2, 'A', now(), now()", org, prj))
				users = append(users, b.row(t, "end_users", "org_id, project_id, created_at, updated_at", "$3, $4, now(), now()", org, prj))
				stores = append(stores, b.row(t, "memory_stores", "org_id, project_id, name, backend, created_at, updated_at", "$3, $4, $2, 'native', now(), now()", org, prj))
			}
			for _, st := range stores {
				for _, a := range agents {
					for _, u := range users {
						out = append(out, memorytest.Cell{Ctx: ctx, Scope: memory.Scope{OrgID: org, ProjectID: prj, StoreID: st, AgentID: a, EndUserID: u}})
					}
				}
			}
		}
	}
	return out
}

func (b *backend) Run(t *testing.T, c memorytest.Cell) uuid.UUID {
	t.Helper()
	s := c.Scope
	version := b.row(t, "agent_versions", "org_id, project_id, agent_id, version, config, created_at, kind", "$3, $4, $5, 1, '{}', now(), 'published'",
		s.OrgID, s.ProjectID, s.AgentID)
	thread := b.row(t, "threads", "org_id, project_id, agent_id, kind, external_key, created_at, updated_at", "$3, $4, $5, 'api', $2, now(), now()",
		s.OrgID, s.ProjectID, s.AgentID)
	return b.row(t, "runs", "org_id, project_id, thread_id, agent_version_id, status, started_at, created_at, updated_at",
		"$3, $4, $5, $6, 'running', now(), now(), now()", s.OrgID, s.ProjectID, thread, version)
}

func (b *backend) Seed(t *testing.T, c memorytest.Cell, texts ...string) {
	t.Helper()
	s := c.Scope
	for _, text := range texts {
		b.seeded++
		b.exec(t, `INSERT INTO memory_facts (id, public_id, org_id, project_id, store_id, agent_id, end_user_id, text, text_hash, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now() - interval '1 hour' + make_interval(secs => $10::float8 / 1000))`,
			uuid.New(), nanoid.MustNew(21), s.OrgID, s.ProjectID, s.StoreID, s.AgentID, s.EndUserID, text, memory.Hash(text), b.seeded)
	}
}

func TestNative_KeepsTheFactsContract(t *testing.T) {
	memorytest.Run(t, func(t *testing.T) memorytest.Backend { return newBackend(t) })
}

func (b *backend) facts(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, b.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM memory_facts`).Scan(&n))
	return n
}

func TestNative_DeletesGoWithTheirOwners(t *testing.T) {
	b := newBackend(t)
	cells := b.Matrix(t)
	c := cells[0]
	for _, cell := range cells[:4] {
		require.NoError(t, b.Facts().Record(cell.Ctx, cell.Scope, memory.Turn{Facts: []string{"Lives in Bandung."}}))
	}
	require.Equal(t, 4, b.facts(t))
	b.exec(t, `DELETE FROM end_users WHERE id = $1`, c.Scope.EndUserID)
	assert.Equal(t, 2, b.facts(t), "a deleted end user takes their facts under every agent")
	b.exec(t, `DELETE FROM agents WHERE id = $1`, cells[3].Scope.AgentID)
	assert.Equal(t, 1, b.facts(t), "a deleted agent takes its facts")
	b.exec(t, `DELETE FROM memory_stores WHERE id = $1`, c.Scope.StoreID)
	assert.Zero(t, b.facts(t), "a deleted store takes its facts")
}

func TestNative_ADeletedRunLeavesTheFact(t *testing.T) {
	b := newBackend(t)
	c := b.Matrix(t)[0]
	run := b.Run(t, c)
	require.NoError(t, b.Facts().Record(c.Ctx, c.Scope, memory.Turn{RunID: run, Facts: []string{"Lives in Bandung."}}))
	b.exec(t, `DELETE FROM runs WHERE id = $1`, run)
	var source sql.NullString
	require.NoError(t, b.admin.QueryRowContext(t.Context(), `SELECT source_run_id FROM memory_facts`).Scan(&source))
	assert.False(t, source.Valid, "the fact stays with no source run")
}

func TestNative_TextLengthIsChecked(t *testing.T) {
	b := newBackend(t)
	c := b.Matrix(t)[0]
	s := c.Scope
	_, err := b.admin.ExecContext(t.Context(), `INSERT INTO memory_facts (id, public_id, org_id, project_id, store_id, agent_id, end_user_id, text, text_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', '\x01', now())`, uuid.New(), "f"+uuid.NewString()[:12], s.OrgID, s.ProjectID, s.StoreID, s.AgentID, s.EndUserID)
	assert.Error(t, err, "an empty fact is refused")
}
