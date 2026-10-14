//go:build integration

package boot

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestBuildServices_OrgCreateAddsTheDefaultProject(t *testing.T) {
	cfg := devLocalConfig()
	cfg.Mode = config.ModeCloud
	svcs, _ := migratedServices(t, cfg)
	ctx := t.Context()

	o, err := svcs.Orgs.Create(ctx, org.CreateRequest{Slug: "acme", Name: "Acme"})
	require.NoError(t, err)

	p, err := svcs.Projects.BySlug(ctx, o.ID, project.DefaultSlug)
	require.NoError(t, err)
	assert.Equal(t, project.DefaultName, p.Name)
	assert.Equal(t, o.ID, p.OrgID)

	ids, err := projectIDs{projects: svcs.Projects}.ProjectIDs(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{p.ID}, ids)
}

func TestBuildServices_AnOrgIsNotCreatedWithoutItsDefaultProject(t *testing.T) {
	cfg := devLocalConfig()
	cfg.Mode = config.ModeCloud
	sqlDB, schemaName := pgtest.Migrated(t)
	cfg.Postgres.Schema = schemaName
	svcs := buildServices(kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}, Reporter: apperror.NewReporter(nil, false)}, cfg)
	ctx := t.Context()
	_, err := sqlDB.ExecContext(ctx, `ALTER TABLE `+schemaName+`.projects ADD CONSTRAINT refuse_default CHECK (slug <> 'default')`)
	require.NoError(t, err)

	_, err = svcs.Orgs.Create(ctx, org.CreateRequest{Slug: "acme", Name: "Acme"})
	require.Error(t, err)
	_, err = svcs.Orgs.BySlug(ctx, "acme")
	assert.True(t, org.IsNotFoundError(err), "the org rolls back with its default project")
}
