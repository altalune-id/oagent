//go:build integration

package boot

import (
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
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
