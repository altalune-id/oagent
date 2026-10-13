//go:build integration

package install_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

type installFixture struct {
	pool     db.Pool
	svc      *install.Service
	users    *user.Service
	orgs     *org.Service
	members  *member.Service
	projects *project.Service
}

func newInstallFixture(t *testing.T) *installFixture {
	t.Helper()
	return newInstallFixtureWithConns(t, 0)
}

func newInstallFixtureWithConns(t *testing.T, maxOpen int) *installFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	sqlDB.SetMaxOpenConns(maxOpen)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	unexpected := apperror.NewReporter(nil, false).Unexpected
	f := &installFixture{
		pool:     pool,
		users:    user.NewService(user.NewStore(pool, schemaName, ""), unexpected),
		orgs:     org.NewService(org.NewStore(pool, schemaName, ""), db.NewTransactor(pool), unexpected).WithCreation(false),
		members:  member.NewService(member.NewStore(pool, schemaName, ""), db.NewTransactor(pool), unexpected),
		projects: project.NewService(project.NewStore(pool, schemaName, ""), unexpected),
	}
	f.svc = install.NewService(db.NewTransactor(pool), f.users, f.orgs, f.members, f.projects).WithTerms(legal.NewService(f.users))
	return f
}

func (f *installFixture) orgCount(t *testing.T) int {
	t.Helper()
	all, err := f.orgs.List(t.Context())
	require.NoError(t, err)
	return len(all)
}

func TestInstall_Fresh(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	required, err := f.svc.Required(ctx, false)
	require.NoError(t, err)
	assert.True(t, required)

	u, err := f.svc.Install(ctx, install.Request{Name: "Ada", Email: "Ada@Example.com", Password: "a-good-password", OrgName: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, "ada@example.com", u.Email)

	sys, err := f.orgs.System(ctx)
	require.NoError(t, err)
	assert.True(t, sys.IsSystem)
	assert.Equal(t, "Acme", sys.Name)
	assert.Equal(t, "default", sys.Slug)
	assert.Equal(t, u.ID, sys.CreatedBy.UUID)

	m, err := f.members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	assert.Nil(t, u.TermsAcceptedAt, "no box ticked, no acceptance")
	_, err = f.projects.BySlug(ctx, sys.ID, project.DefaultSlug)
	require.NoError(t, err)

	required, err = f.svc.Required(ctx, false)
	require.NoError(t, err)
	assert.False(t, required)

	_, err = f.svc.Install(ctx, install.Request{Email: "eve@example.com", Password: "a-good-password"})
	assert.True(t, install.IsAlreadyInstalledError(err))
	_, err = f.users.ByEmail(ctx, "eve@example.com")
	assert.True(t, user.IsNotFoundError(err), "a refused install writes nothing")
}

func TestInstall_ConcurrentOnlyOneWins(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	const n = 4
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			_, errs[i] = f.svc.Install(ctx, install.Request{Email: "user" + string(rune('a'+i)) + "@example.com", Password: "a-good-password"})
		})
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.True(t, install.IsAlreadyInstalledError(err), err)
	}
	assert.Equal(t, 1, wins)
	sys, err := f.orgs.System(ctx)
	require.NoError(t, err)
	owners, err := f.members.CountOwners(ctx, sys.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, owners)
	assert.Equal(t, 1, f.orgCount(t))
}

func TestInstall_ConcurrentOnASmallPoolDoesNotDeadlock(t *testing.T) {
	f := newInstallFixtureWithConns(t, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	const n = 3
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			_, errs[i] = f.svc.Install(ctx, install.Request{Email: fmt.Sprintf("user%d@example.com", i), Password: "a-good-password"})
		})
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.True(t, install.IsAlreadyInstalledError(err), err)
	}
	assert.Equal(t, 1, wins)
}

func TestInstall_ClaimsBasicSystemOrg(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	require.NoError(t, f.svc.EnsureBasicSystemOrg(ctx))
	require.NoError(t, f.svc.EnsureBasicSystemOrg(ctx))
	assert.Equal(t, 1, f.orgCount(t))
	sys, err := f.orgs.System(ctx)
	require.NoError(t, err)
	assert.False(t, sys.CreatedBy.Valid)
	owners, err := f.members.CountOwners(ctx, sys.ID)
	require.NoError(t, err)
	assert.Zero(t, owners)
	_, err = f.projects.BySlug(ctx, sys.ID, project.DefaultSlug)
	require.NoError(t, err)

	required, err := f.svc.Required(ctx, true)
	require.NoError(t, err)
	assert.False(t, required)
	required, err = f.svc.Required(ctx, false)
	require.NoError(t, err)
	assert.True(t, required, "switching basic to local still needs an owner")

	u, err := f.svc.Install(ctx, install.Request{Email: "ada@example.com", Password: "a-good-password", OrgName: "Ignored"})
	require.NoError(t, err)
	claimed, err := f.orgs.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, sys.ID, claimed.ID)
	assert.Equal(t, 1, f.orgCount(t))
	m, err := f.members.Find(ctx, sys.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	all, err := f.projects.List(ctx, sys.ID)
	require.NoError(t, err)
	assert.Len(t, all, 1, "the default project is not duplicated")
}

func TestInstall_WeakPasswordWritesNothing(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	_, err := f.svc.Install(ctx, install.Request{Email: "ada@example.com", Password: "short"})
	assert.True(t, password.IsPolicyError(err))
	assert.Zero(t, f.orgCount(t))
	_, err = f.users.ByEmail(ctx, "ada@example.com")
	assert.True(t, user.IsNotFoundError(err))
}

func TestInstall_FailureAfterUserRollsBack(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	_, err := f.svc.Install(ctx, install.Request{Email: "ada@example.com", Password: "a-good-password", OrgName: strings.Repeat("x", 500)})
	assert.True(t, org.IsInvalidNameError(err))
	_, err = f.users.ByEmail(ctx, "ada@example.com")
	assert.True(t, user.IsNotFoundError(err), "the user insert is rolled back with the failed org")
	assert.Zero(t, f.orgCount(t))
}

var errInjected = errors.New("injected failure")

type failSystem struct{ install.SystemOrgs }

func (failSystem) System(context.Context) (*org.Org, error) { return nil, errInjected }

type failCount struct{ install.Owners }

func (failCount) CountOwners(context.Context, uuid.UUID) (int, error) { return 0, errInjected }

type failAdd struct{ install.Owners }

func (failAdd) Add(context.Context, uuid.UUID, uuid.UUID, member.Role) (*member.Membership, error) {
	return nil, errInjected
}

type failProjectCreate struct{ install.DefaultProjects }

func (failProjectCreate) Create(context.Context, project.CreateRequest) (*project.Project, error) {
	return nil, errInjected
}

func TestInstall_DependencyFailuresRollBack(t *testing.T) {
	cases := []struct {
		name  string
		basic bool
		wrap  func(f *installFixture, pool db.Pool) *install.Service
	}{
		{"system org lookup", false, func(f *installFixture, pool db.Pool) *install.Service {
			return install.NewService(db.NewTransactor(pool), f.users, failSystem{f.orgs}, f.members, f.projects)
		}},
		{"owner count", true, func(f *installFixture, pool db.Pool) *install.Service {
			return install.NewService(db.NewTransactor(pool), f.users, f.orgs, failCount{f.members}, f.projects)
		}},
		{"owner membership", false, func(f *installFixture, pool db.Pool) *install.Service {
			return install.NewService(db.NewTransactor(pool), f.users, f.orgs, failAdd{f.members}, f.projects)
		}},
		{"default project", false, func(f *installFixture, pool db.Pool) *install.Service {
			return install.NewService(db.NewTransactor(pool), f.users, f.orgs, f.members, failProjectCreate{f.projects})
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newInstallFixture(t)
			ctx := t.Context()
			if tt.basic {
				require.NoError(t, f.svc.EnsureBasicSystemOrg(ctx))
			}
			before := f.orgCount(t)

			_, err := tt.wrap(f, f.pool).Install(ctx, install.Request{Email: "ada@example.com", Password: "a-good-password"})

			require.ErrorIs(t, err, errInjected)
			_, err = f.users.ByEmail(ctx, "ada@example.com")
			assert.True(t, user.IsNotFoundError(err), "the user insert is rolled back")
			assert.Equal(t, before, f.orgCount(t))
		})
	}
}

func TestEnsureBasicSystemOrg_DependencyFailures(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()

	err := install.NewService(db.NewTransactor(f.pool), f.users, failSystem{f.orgs}, f.members, f.projects).EnsureBasicSystemOrg(ctx)
	require.ErrorIs(t, err, errInjected)

	err = install.NewService(db.NewTransactor(f.pool), f.users, f.orgs, f.members, failProjectCreate{f.projects}).EnsureBasicSystemOrg(ctx)
	require.ErrorIs(t, err, errInjected)
	assert.Zero(t, f.orgCount(t), "the system org insert is rolled back with the failed project")
}

func TestInstall_RecordsAcceptedTerms(t *testing.T) {
	f := newInstallFixture(t)
	ctx := t.Context()
	before := time.Now().Add(-time.Second)
	u, err := f.svc.Install(ctx, install.Request{Email: "ada@example.com", Password: "a-good-password", AcceptTerms: true})
	require.NoError(t, err)
	stored, err := f.users.ByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.TermsAcceptedAt)
	assert.True(t, stored.TermsAcceptedAt.After(before))
}
