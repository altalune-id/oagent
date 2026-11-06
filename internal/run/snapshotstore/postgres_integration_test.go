//go:build integration

package snapshotstore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
	"altalune.id/oagent/internal/run/snapshotstoretest"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestPostgres_Contract(t *testing.T) {
	snapshotstoretest.Run(t, func(t *testing.T) snapshotstoretest.Harness {
		pool, admin, schema := pgtest.MigratedRLS(t)
		return snapshotstoretest.Harness{
			Store: snapshotstore.NewPostgres(pool, schema, ""),
			Thread: func(t *testing.T) (context.Context, run.Snapshot) {
				t.Helper()
				org, project, agent, version, thread := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
				p := schema + "."
				for _, q := range []struct {
					sql  string
					args []any
				}{
					{`INSERT INTO ` + p + `orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'O', now(), now())`, []any{org, "o" + org.String()[:12]}},
					{`INSERT INTO ` + p + `projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', now(), now())`, []any{project, "p" + project.String()[:12], org}},
					{`INSERT INTO ` + p + `agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, 'a', 'A', now(), now())`, []any{agent, "a" + agent.String()[:12], org, project}},
					{`INSERT INTO ` + p + `agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at) VALUES ($1, $2, $3, $4, $5, 1, '{}', now())`, []any{version, "v" + version.String()[:12], org, project, agent}},
					{`INSERT INTO ` + p + `threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 'api', 'k', now(), now())`, []any{thread, "t" + thread.String()[:12], org, project, agent}},
				} {
					_, err := admin.ExecContext(t.Context(), q.sql, q.args...)
					require.NoError(t, err)
				}
				ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org})
				return ctx, run.Snapshot{ThreadID: thread, OrgID: org, ProjectID: project, AgentVersionID: version}
			},
		}
	})
}
