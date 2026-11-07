//go:build integration

package enduser_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type pgFixture struct {
	admin    *sql.DB
	schema   string
	pool     db.Pool
	store    enduser.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &pgFixture{
		admin:    admin,
		schema:   schemaName,
		pool:     pool,
		store:    enduser.NewStore(pool, schemaName, ""),
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func in(orgID uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: orgID})
}

func (f *pgFixture) seedScope(t *testing.T) (orgID, projectID uuid.UUID) {
	t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(in(o.ID), o))
	return o.ID, f.seedProject(t, o.ID)
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:8], "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(in(orgID), p))
	return p.ID
}

func (f *pgFixture) service() *enduser.Service {
	return enduser.NewService(f.store, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected)
}

func TestStore_UniqueIdentityPerProject(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service()

	first, err := svc.Upsert(ctx, projectID, phone)
	require.NoError(t, err)
	other := enduser.New(orgID, projectID, first.CreatedAt)
	require.NoError(t, f.store.Save(ctx, other))
	added, err := f.store.AddIdentity(ctx, other, phone)
	require.NoError(t, err)
	assert.False(t, added, "a known identity is not linked a second time")
	owners, err := f.store.Owners(ctx, projectID, []enduser.Identity{phone})
	require.NoError(t, err)
	assert.Equal(t, first.ID, owners[phone])

	again, err := svc.Upsert(ctx, projectID, phone, lid)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)

	otherProject := f.seedProject(t, orgID)
	elsewhere, err := svc.Upsert(ctx, otherProject, phone)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, elsewhere.ID, "the same identity in another project is another end user")
}

func TestStore_OtherTenantInvisible(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, _ := f.seedScope(t)
	a, err := f.service().Upsert(in(orgA), projectA, phone)
	require.NoError(t, err)

	_, err = f.store.ByID(in(orgB), a.ID)
	assert.True(t, enduser.IsNotFoundError(err))
	owners, err := f.store.Owners(in(orgB), projectA, []enduser.Identity{phone})
	require.NoError(t, err)
	assert.Empty(t, owners)

	_, err = f.service().Upsert(in(orgB), projectA, lid)
	assert.Error(t, err, "another org cannot create end users in this project")
	_, err = f.store.AddIdentity(in(orgB), a, lid)
	assert.True(t, enduser.IsNotFoundError(err))
}

func TestStore_IdentitiesOfOneEndUser(t *testing.T) {
	f := newPgFixture(t)
	org, project := f.seedScope(t)
	other, _ := f.seedScope(t)
	e, err := f.service().Upsert(in(org), project, phone, lid)
	require.NoError(t, err)
	got, err := f.store.IdentitiesOf(in(org), e.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []enduser.Identity{phone, lid}, got)
	foreign, err := f.store.IdentitiesOf(in(other), e.ID)
	require.NoError(t, err)
	assert.Empty(t, foreign, "another tenant sees nothing")
}

func TestStore_ConcurrentUpsertOneEndUser(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service()

	var (
		wg                 sync.WaitGroup
		once               sync.Once
		first, second      *enduser.EndUser
		holdErr, secondErr error
		held               = make(chan struct{})
		release            = make(chan struct{})
	)
	wg.Go(func() {
		defer once.Do(func() { close(held) })
		holdErr = db.NewTransactor(f.pool).RunInTx(ctx, func(ctx context.Context) error {
			first = enduser.New(orgID, projectID, time.Now())
			if err := f.store.Save(ctx, first); err != nil {
				return err
			}
			if _, err := f.store.AddIdentity(ctx, first, phone); err != nil {
				return err
			}
			once.Do(func() { close(held) })
			<-release
			return nil
		})
	})
	<-held
	require.NoError(t, holdErr)
	wg.Go(func() { second, secondErr = svc.Upsert(ctx, projectID, phone) })
	require.Eventually(t, func() bool {
		var waiting int
		err := f.admin.QueryRowContext(ctx,
			"SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%' || $1 || '.end_user_identities%'",
			f.schema).Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 10*time.Millisecond, "the second upsert waits on the first one's identity")
	close(release)
	wg.Wait()

	require.NoError(t, holdErr)
	require.NoError(t, secondErr)
	assert.Equal(t, first.ID, second.ID, "the loser resolves to the winner's end user")
	var users int
	require.NoError(t, f.admin.QueryRowContext(ctx, "SELECT count(*) FROM "+f.schema+".end_users WHERE project_id = $1", projectID).Scan(&users))
	assert.Equal(t, 1, users, "the loser's end user rolls back")
}

func TestUpsert_RefusesACallersTransaction(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	err := db.NewTransactor(f.pool).RunInTx(in(orgID), func(ctx context.Context) error {
		_, err := f.service().Upsert(ctx, projectID, phone)
		return err
	})
	assert.Error(t, err)
	var users int
	require.NoError(t, f.admin.QueryRowContext(t.Context(), "SELECT count(*) FROM "+f.schema+".end_users WHERE project_id = $1", projectID).Scan(&users))
	assert.Zero(t, users)
}

func TestUpsert_RepeatedIdentityAgainstPostgres(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	e, err := f.service().Upsert(in(orgID), projectID, phone, lid, phone)
	require.NoError(t, err, "a repeated identity that is not adjacent is still one identity")
	owners, err := f.store.Owners(in(orgID), projectID, []enduser.Identity{phone, lid})
	require.NoError(t, err)
	assert.Equal(t, e.ID, owners[phone])
	assert.Equal(t, e.ID, owners[lid])
}

func TestStore_DisplayNameSetAndRead(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	e, err := f.service().Upsert(in(orgID), projectID, enduser.Identity{Kind: enduser.KindWAPhone, Value: "628111222333"})
	require.NoError(t, err)
	assert.Empty(t, e.DisplayName)
	require.NoError(t, f.store.SetDisplayName(in(orgID), e.ID, "Budi"))
	got, err := f.store.ByID(in(orgID), e.ID)
	require.NoError(t, err)
	assert.Equal(t, "Budi", got.DisplayName)
	require.NoError(t, f.store.SetDisplayName(in(orgID), e.ID, ""))
	got, err = f.store.ByID(in(orgID), e.ID)
	require.NoError(t, err)
	assert.Empty(t, got.DisplayName, "an empty name clears it")
	other, _ := f.seedScope(t)
	assert.True(t, enduser.IsNotFoundError(f.store.SetDisplayName(in(other), e.ID, "x")), "another org changes nothing")
}

func TestStore_ResolveListAndIdentitiesOfMany(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service()
	sari, err := svc.Upsert(ctx, projectID, enduser.Identity{Kind: enduser.KindWAPhone, Value: "6281234567890"})
	require.NoError(t, err)
	require.NoError(t, svc.SetDisplayName(ctx, sari.ID, "Sari Wulandari"))
	budi, err := svc.Upsert(ctx, projectID, enduser.Identity{Kind: enduser.KindWAPhone, Value: "6289876543210"})
	require.NoError(t, err)
	f.exec(t, `UPDATE `+f.schema+`.end_users SET created_at = now() - interval '1 hour' WHERE id = $1`, sari.ID)

	ref, err := f.store.Resolve(context.Background(), budi.PublicID)
	require.NoError(t, err, "resolve needs no tenant")
	assert.Equal(t, enduser.Ref{ID: budi.ID, OrgID: orgID, ProjectID: projectID}, ref)
	_, err = f.store.Resolve(context.Background(), "unknwnunknwnunknwnabc")
	assert.True(t, enduser.IsNotFoundError(err))

	all, err := f.store.List(ctx, projectID, "", enduser.Cursor{}, 10)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, budi.ID, all[0].ID, "newest first")
	page, err := f.store.List(ctx, projectID, "", enduser.Cursor{At: all[0].CreatedAt, PublicID: all[0].PublicID}, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, sari.ID, page[0].ID)
	found, err := f.store.List(ctx, projectID, "wulan", enduser.Cursor{}, 10)
	require.NoError(t, err)
	require.Len(t, found, 1, "the query matches part of a display name, any case")
	assert.Equal(t, sari.ID, found[0].ID)
	byNumber, err := f.store.List(ctx, projectID, "6289876543210", enduser.Cursor{}, 10)
	require.NoError(t, err)
	require.Len(t, byNumber, 1, "a whole identity value finds its person")
	assert.Equal(t, budi.ID, byNumber[0].ID)
	none, err := f.store.List(ctx, projectID, "62898", enduser.Cursor{}, 10)
	require.NoError(t, err)
	assert.Empty(t, none, "part of a number finds nobody")

	ids, err := f.store.IdentitiesOfMany(ctx, []uuid.UUID{sari.ID, budi.ID})
	require.NoError(t, err)
	assert.Equal(t, []enduser.Identity{{Kind: enduser.KindWAPhone, Value: "6281234567890"}}, ids[sari.ID])
	assert.Len(t, ids[budi.ID], 1)
}

func (f *pgFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := f.admin.ExecContext(t.Context(), q, args...)
	require.NoError(t, err)
}
