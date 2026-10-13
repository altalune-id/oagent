//go:build integration

package member_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

type memberFixture struct {
	pool   db.Pool
	schema string
	store  member.Store
	users  user.Store
	orgs   org.Store
	ctx    context.Context
	t      *testing.T
}

func newMemberFixture(t *testing.T) *memberFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &memberFixture{
		pool: pool, schema: schemaName, ctx: t.Context(), t: t,
		store: member.NewStore(pool, schemaName, ""),
		users: user.NewStore(pool, schemaName, ""),
		orgs:  org.NewStore(pool, schemaName, ""),
	}
}

func (f *memberFixture) user() *user.User {
	f.t.Helper()
	u, err := user.New(uuid.NewString()+"@example.com", "")
	require.NoError(f.t, err)
	require.NoError(f.t, f.users.Save(f.ctx, u))
	return u
}

func (f *memberFixture) org() *org.Org {
	f.t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(f.t, err)
	require.NoError(f.t, f.orgs.Save(f.ctx, o))
	return o
}

func (f *memberFixture) add(orgID, userID uuid.UUID, role member.Role) *member.Membership {
	f.t.Helper()
	m, err := member.New(orgID, userID, role)
	require.NoError(f.t, err)
	require.NoError(f.t, f.store.Add(f.ctx, m))
	return m
}

func pgCode(err error) string {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return ""
	}
	return pgErr.Code
}

func TestPostgresMember_AddFind(t *testing.T) {
	f := newMemberFixture(t)
	u, o := f.user(), f.org()
	m := f.add(o.ID, u.ID, member.RoleAdmin)

	got, err := f.store.Find(f.ctx, o.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, m.PublicID, got.PublicID)
	assert.Equal(t, member.RoleAdmin, got.Role)
	assert.True(t, m.CreatedAt.Truncate(time.Microsecond).Equal(got.CreatedAt))

	_, err = f.store.Find(f.ctx, o.ID, f.user().ID)
	assert.True(t, member.IsNotFoundError(err))
	_, err = f.store.Find(f.ctx, f.org().ID, u.ID)
	assert.True(t, member.IsNotFoundError(err), "membership is per org")
}

func TestPostgresMember_OnePerOrgAndUser(t *testing.T) {
	f := newMemberFixture(t)
	u, o := f.user(), f.org()
	first := f.add(o.ID, u.ID, member.RoleOwner)

	again, err := member.New(o.ID, u.ID, member.RoleMember)
	require.NoError(t, err)
	err = f.store.Add(f.ctx, again)
	require.True(t, member.IsAlreadyMemberError(err), "got %v", err)
	got, err := f.store.Find(f.ctx, o.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, first.ID, got.ID, "the first membership is kept")
	assert.Equal(t, member.RoleOwner, got.Role)

	dupPublic, err := member.New(f.org().ID, u.ID, member.RoleMember)
	require.NoError(t, err)
	dupPublic.PublicID = first.PublicID
	assert.True(t, member.IsAlreadyMemberError(f.store.Add(f.ctx, dupPublic)), "public_id is unique too")

	f.add(f.org().ID, u.ID, member.RoleMember)
}

func TestPostgresMember_ListByUserOldestFirst(t *testing.T) {
	f := newMemberFixture(t)
	u, other := f.user(), f.user()
	o1, o2, o3 := f.org(), f.org(), f.org()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, o := range []*org.Org{o2, o3, o1} {
		m, err := member.New(o.ID, u.ID, member.RoleMember)
		require.NoError(t, err)
		m.CreatedAt = base.Add(time.Duration(i) * time.Hour)
		require.NoError(t, f.store.Add(f.ctx, m))
	}
	f.add(o1.ID, other.ID, member.RoleOwner)

	got, err := f.store.ListByUser(f.ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []uuid.UUID{o2.ID, o3.ID, o1.ID}, []uuid.UUID{got[0].OrgID, got[1].OrgID, got[2].OrgID})

	none, err := f.store.ListByUser(f.ctx, uuid.New())
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

func TestPostgresMember_CountOwners(t *testing.T) {
	f := newMemberFixture(t)
	o, other := f.org(), f.org()
	f.add(o.ID, f.user().ID, member.RoleOwner)
	f.add(o.ID, f.user().ID, member.RoleOwner)
	f.add(o.ID, f.user().ID, member.RoleAdmin)
	f.add(other.ID, f.user().ID, member.RoleOwner)

	n, err := f.store.CountOwners(f.ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = f.store.CountOwners(f.ctx, uuid.New())
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestPostgresMember_Constraints(t *testing.T) {
	f := newMemberFixture(t)
	u, o := f.user(), f.org()

	_, err := f.pool.W.ExecContext(f.ctx,
		`INSERT INTO memberships (id, public_id, org_id, user_id, role, created_at) VALUES ($1, 'p-root', $2, $3, 'root', now())`,
		uuid.New(), o.ID, u.ID)
	assert.Equal(t, "23514", pgCode(err), "role CHECK: %v", err)

	for name, ids := range map[string][2]uuid.UUID{
		"unknown org":  {uuid.New(), u.ID},
		"unknown user": {o.ID, uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			m, err := member.New(ids[0], ids[1], member.RoleMember)
			require.NoError(t, err)
			err = f.store.Add(f.ctx, m)
			assert.Equal(t, "23503", pgCode(err), "got %v", err)
			assert.False(t, member.IsExpectedError(err))
		})
	}
}

func TestPostgresMember_Cascades(t *testing.T) {
	f := newMemberFixture(t)
	gone, kept := f.user(), f.user()
	o := f.org()
	f.add(o.ID, gone.ID, member.RoleOwner)
	f.add(o.ID, kept.ID, member.RoleMember)

	_, err := f.pool.W.ExecContext(f.ctx, `DELETE FROM users WHERE id = $1`, gone.ID)
	require.NoError(t, err)
	_, err = f.store.Find(f.ctx, o.ID, gone.ID)
	assert.True(t, member.IsNotFoundError(err), "deleting a user removes their memberships")
	_, err = f.store.Find(f.ctx, o.ID, kept.ID)
	require.NoError(t, err)

	_, err = f.pool.W.ExecContext(f.ctx, `DELETE FROM orgs WHERE id = $1`, o.ID)
	require.NoError(t, err)
	_, err = f.store.Find(f.ctx, o.ID, kept.ID)
	assert.True(t, member.IsNotFoundError(err), "deleting an org removes its memberships")
}

func TestPostgresMember_HonorsTransaction(t *testing.T) {
	f := newMemberFixture(t)
	u, o := f.user(), f.org()
	errAbort := errors.New("abort")
	err := db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
		m, err := member.New(o.ID, u.ID, member.RoleOwner)
		require.NoError(t, err)
		require.NoError(t, f.store.Add(ctx, m))
		_, err = f.store.Find(ctx, o.ID, u.ID)
		require.NoError(t, err, "visible inside the transaction")
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.Find(f.ctx, o.ID, u.ID)
	assert.True(t, member.IsNotFoundError(err), "rolled back")
}

func TestPostgresMember_LockOwnersNeedsATransaction(t *testing.T) {
	f := newMemberFixture(t)
	o := f.org()
	a, b := f.user(), f.user()
	f.add(o.ID, a.ID, member.RoleOwner)
	f.add(o.ID, b.ID, member.RoleOwner)
	f.add(o.ID, f.user().ID, member.RoleAdmin)
	f.add(f.org().ID, f.user().ID, member.RoleOwner)

	_, err := f.store.LockOwners(f.ctx, o.ID)
	require.Error(t, err, "outside a transaction the lock would end at once")

	err = db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
		owners, err := f.store.LockOwners(ctx, o.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{a.ID, b.ID}, owners)
		return nil
	})
	require.NoError(t, err)
}

func TestPostgresMember_LockOwnersBlocksASecondLocker(t *testing.T) {
	f := newMemberFixture(t)
	o := f.org()
	f.add(o.ID, f.user().ID, member.RoleOwner)

	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan time.Time)
	go func() {
		_ = db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
			_, err := f.store.LockOwners(ctx, o.ID)
			close(locked)
			<-release
			return err
		})
	}()
	<-locked
	go func() {
		_ = db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
			_, err := f.store.LockOwners(ctx, o.ID)
			done <- time.Now()
			return err
		})
	}()
	time.Sleep(100 * time.Millisecond)
	released := time.Now()
	close(release)
	assert.True(t, (<-done).After(released), "the second locker waits for the first")
}

func TestPostgresMember_LockMember(t *testing.T) {
	f := newMemberFixture(t)
	o := f.org()
	admin := f.user()
	f.add(o.ID, admin.ID, member.RoleAdmin)

	_, err := f.store.LockMember(f.ctx, o.ID, admin.ID)
	require.Error(t, err, "outside a transaction the lock would end at once")

	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan time.Time)
	go func() {
		_ = db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
			m, err := f.store.LockMember(ctx, o.ID, admin.ID)
			assert.NoError(t, err)
			assert.Equal(t, member.RoleAdmin, m.Role)
			_, err = f.store.LockMember(ctx, o.ID, uuid.New())
			assert.True(t, member.IsNotFoundError(err), "got %v", err)
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	go func() {
		_ = db.RunInTx(f.ctx, f.pool, func(ctx context.Context) error {
			err := f.store.UpdateRole(ctx, o.ID, admin.ID, member.RoleMember)
			done <- time.Now()
			return err
		})
	}()
	time.Sleep(100 * time.Millisecond)
	released := time.Now()
	close(release)
	assert.True(t, (<-done).After(released), "a role change waits for the share lock")
}

func TestPostgresMember_UpdateRoleAndDelete(t *testing.T) {
	f := newMemberFixture(t)
	o := f.org()
	u, kept := f.user(), f.user()
	f.add(o.ID, u.ID, member.RoleMember)
	f.add(o.ID, kept.ID, member.RoleMember)
	f.add(f.org().ID, u.ID, member.RoleMember)

	require.NoError(t, f.store.UpdateRole(f.ctx, o.ID, u.ID, member.RoleAdmin))
	got, err := f.store.Find(f.ctx, o.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, got.Role)
	other, err := f.store.Find(f.ctx, o.ID, kept.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleMember, other.Role, "only that row changes")

	require.NoError(t, f.store.Delete(f.ctx, o.ID, u.ID))
	_, err = f.store.Find(f.ctx, o.ID, u.ID)
	assert.True(t, member.IsNotFoundError(err))
	list, err := f.store.ListByUser(f.ctx, u.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1, "their other org is untouched")

	assert.True(t, member.IsNotFoundError(f.store.UpdateRole(f.ctx, o.ID, u.ID, member.RoleAdmin)))
	assert.True(t, member.IsNotFoundError(f.store.Delete(f.ctx, o.ID, u.ID)))
}

func TestPostgresMember_ListByOrg(t *testing.T) {
	f := newMemberFixture(t)
	o := f.org()
	named := func(email, name string) *user.User {
		u, err := user.New(email, name)
		require.NoError(t, err)
		require.NoError(t, f.users.Save(f.ctx, u))
		return u
	}
	suffix := uuid.NewString()[:8]
	m := named("a-"+suffix+"@example.com", "Ann")
	z := named("z-"+suffix+"@example.com", "Zed")
	c := named("c-"+suffix+"@example.com", "Cat")
	b := named("b-"+suffix+"@example.com", "")
	f.add(o.ID, m.ID, member.RoleMember)
	f.add(o.ID, z.ID, member.RoleOwner)
	f.add(o.ID, c.ID, member.RoleAdmin)
	f.add(o.ID, b.ID, member.RoleOwner)
	f.add(f.org().ID, f.user().ID, member.RoleOwner)

	got, err := f.store.ListByOrg(f.ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, got, 4)
	ids := []uuid.UUID{got[0].UserID, got[1].UserID, got[2].UserID, got[3].UserID}
	assert.Equal(t, []uuid.UUID{b.ID, z.ID, c.ID, m.ID}, ids, "owner, admin, member, then email")
	assert.Equal(t, z.Email, got[1].Email)
	assert.Equal(t, "Zed", got[1].Name)
	assert.Empty(t, got[0].Name)
	assert.Equal(t, member.RoleAdmin, got[2].Role)
	assert.Equal(t, o.ID, got[2].OrgID)
	assert.False(t, got[2].CreatedAt.IsZero())

	none, err := f.store.ListByOrg(f.ctx, uuid.New())
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

func TestPostgresMember_AddIfAbsent(t *testing.T) {
	f := newMemberFixture(t)
	u, o := f.user(), f.org()
	f.add(o.ID, u.ID, member.RoleOwner)

	again, err := member.New(o.ID, u.ID, member.RoleMember)
	require.NoError(t, err)
	added, err := f.store.AddIfAbsent(f.ctx, again)
	require.NoError(t, err)
	assert.False(t, added)
	got, err := f.store.Find(f.ctx, o.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, got.Role, "the existing role is kept")

	fresh, err := member.New(o.ID, f.user().ID, member.RoleMember)
	require.NoError(t, err)
	added, err = f.store.AddIfAbsent(f.ctx, fresh)
	require.NoError(t, err)
	assert.True(t, added)

	broken, err := member.New(uuid.New(), u.ID, member.RoleMember)
	require.NoError(t, err)
	_, err = f.store.AddIfAbsent(f.ctx, broken)
	assert.Equal(t, "23503", pgCode(err), "an unknown org is a foreign key failure")
}
