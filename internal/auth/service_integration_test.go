//go:build integration

package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

func TestLoginLocal_AgainstPostgres(t *testing.T) {
	ctx := t.Context()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	unexpected := apperror.NewReporter(nil, false).Unexpected
	orgStore := org.NewStore(pool, schemaName, "")
	users := user.NewService(user.NewStore(pool, schemaName, ""), unexpected)
	members := member.NewService(member.NewStore(pool, schemaName, ""), db.NewTransactor(pool), unexpected)
	orgs := org.NewService(orgStore, db.NewTransactor(pool), unexpected)
	projects := project.NewService(project.NewStore(pool, schemaName, ""), unexpected)
	svc := auth.NewService(auth.Deps{Users: users, Members: members, Orgs: orgs, Projects: projects, Sessions: session.NewMemoryStore()})

	owner, err := users.Create(ctx, user.CreateRequest{Email: "Owner@Example.com", Name: "Owner", Password: "blue-river-stone-42"})
	require.NoError(t, err)
	sys, err := org.NewSystemOrg("default", "Default", owner.ID)
	require.NoError(t, err)
	require.NoError(t, orgStore.Save(ctx, sys))
	_, err = members.Add(ctx, sys.ID, owner.ID, member.RoleOwner)
	require.NoError(t, err)
	defaultPrj, err := projects.Create(ctx, project.CreateRequest{OrgID: sys.ID, Slug: project.DefaultSlug, Name: project.DefaultName})
	require.NoError(t, err)

	p, err := svc.LoginLocal(ctx, auth.Credentials{Email: "owner@example.com", Password: "blue-river-stone-42"})
	require.NoError(t, err)
	assert.True(t, p.Operator)
	assert.Equal(t, []string{"*"}, p.Scopes)
	assert.Equal(t, sys.ID, p.ActiveOrgID)
	assert.Equal(t, defaultPrj.ID, p.ActiveProjectID)

	_, err = svc.LoginLocal(ctx, auth.Credentials{Email: "owner@example.com", Password: "wrong-password"})
	assert.True(t, auth.IsInvalidCredentialsError(err))
	_, err = svc.LoginLocal(ctx, auth.Credentials{Email: "ghost@example.com", Password: "blue-river-stone-42"})
	assert.True(t, auth.IsInvalidCredentialsError(err))

	other, err := orgs.Create(ctx, org.CreateRequest{Name: "Other", CreatedBy: owner.ID})
	require.NoError(t, err)
	worker, err := users.Create(ctx, user.CreateRequest{Email: "worker@example.com", Name: "Worker", Password: "green-hill-cloud-17"})
	require.NoError(t, err)
	_, err = members.Add(ctx, other.ID, worker.ID, member.RoleMember)
	require.NoError(t, err)

	p, err = svc.LoginLocal(ctx, auth.Credentials{Email: "worker@example.com", Password: "green-hill-cloud-17"})
	require.NoError(t, err)
	assert.False(t, p.Operator)
	assert.Equal(t, member.BuiltinPermissions(member.RoleMember), p.Scopes)
	assert.Equal(t, other.ID, p.ActiveOrgID)
}
