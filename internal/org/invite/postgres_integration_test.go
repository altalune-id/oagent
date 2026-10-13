//go:build integration

package invite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

type pgFixture struct {
	t       *testing.T
	ctx     context.Context
	pool    db.Pool
	store   invite.Store
	users   user.Store
	members *member.Service
	svc     *invite.Service
	org     *org.Org
	owner   *user.User
	clock   time.Time
	mu      sync.Mutex
}

type userPeople struct{ users user.Store }

func (p userPeople) PublicID(ctx context.Context, id uuid.UUID) (string, error) {
	u, err := p.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	return u.PublicID, nil
}

func (p userPeople) Contact(ctx context.Context, id uuid.UUID) (string, string, error) {
	u, err := p.users.ByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	return u.Email, u.Name, nil
}

func newPGFixture(t *testing.T) *pgFixture {
	t.Helper()
	sqlDB, schema := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	unexpected := apperror.NewReporter(nil, false).Unexpected
	f := &pgFixture{
		t: t, ctx: t.Context(), pool: pool, store: invite.NewStore(pool, schema, ""),
		users: user.NewStore(pool, schema, ""), clock: time.Now().UTC(),
	}
	memberStore := member.NewStore(pool, schema, "")
	f.members = member.NewService(memberStore, db.NewTransactor(pool), unexpected)
	o, err := org.NewOrg("acme-"+uuid.NewString()[:8], "Acme", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, org.NewStore(pool, schema, "").Save(f.ctx, o))
	f.org = o
	f.owner = f.user("owner-" + uuid.NewString()[:8] + "@example.com")
	_, err = f.members.Add(f.ctx, o.ID, f.owner.ID, member.RoleOwner)
	require.NoError(t, err)
	f.svc = invite.NewService(invite.Deps{
		Store: f.store, Transactor: db.NewTransactor(pool), Members: f.members, Mail: &mailBox{}, Orgs: orgNames{o.ID: "Acme"},
		People: userPeople{users: f.users}, Users: user.NewService(f.users, unexpected), Terms: legal.NewService(user.NewService(f.users, unexpected)), BaseURL: baseURL, Unexpected: unexpected,
	}).WithClock(f.now)
	return f
}

func (f *pgFixture) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clock
}

func (f *pgFixture) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(d)
}

func (f *pgFixture) user(email string) *user.User {
	f.t.Helper()
	u, err := user.New(email, "")
	require.NoError(f.t, err)
	require.NoError(f.t, f.users.Save(f.ctx, u))
	return u
}

func (f *pgFixture) invite(email string, role member.Role) *invite.Created {
	f.t.Helper()
	c, err := f.svc.Create(f.ctx, member.Requester{UserID: f.owner.ID}, f.org.ID, email, role)
	require.NoError(f.t, err)
	return c
}

func race(calls ...func() error) []error {
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, len(calls))
	for i, call := range calls {
		ready.Add(1)
		done.Go(func() {
			ready.Done()
			<-start
			errs[i] = call()
		})
	}
	ready.Wait()
	close(start)
	done.Wait()
	return errs
}

func TestPostgresInvite_DoubleAccept(t *testing.T) {
	for range 5 {
		f := newPGFixture(t)
		bob := f.user("bob-" + uuid.NewString()[:8] + "@example.com")
		c := f.invite(bob.Email, member.RoleMember)

		outcomes := make([]invite.Outcome, 2)
		errs := race(
			func() (err error) { outcomes[0], err = f.svc.Accept(f.ctx, c.Invite.ID, bob.ID); return },
			func() (err error) { outcomes[1], err = f.svc.Accept(f.ctx, c.Invite.ID, bob.ID); return },
		)
		joined, used := 0, 0
		for i, err := range errs {
			switch {
			case err == nil && outcomes[i] == invite.Joined:
				joined++
			case invite.ReasonOf(err) == invite.ReasonUsed:
				used++
			default:
				t.Fatalf("unexpected result: %v, %v", outcomes[i], err)
			}
		}
		assert.Equal(t, 1, joined)
		assert.Equal(t, 1, used)
		ms, err := f.members.List(f.ctx, f.org.ID)
		require.NoError(t, err)
		assert.Len(t, ms, 2, "the owner and bob, once")
	}
}

func TestPostgresInvite_OwnerAcceptsAMemberInvite(t *testing.T) {
	f := newPGFixture(t)
	c := f.invite(f.owner.Email, member.RoleMember)
	got, err := f.svc.Accept(f.ctx, c.Invite.ID, f.owner.ID)
	require.NoError(t, err)
	assert.Equal(t, invite.AlreadyMember, got)
	m, err := f.members.Find(f.ctx, f.org.ID, f.owner.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.AcceptedAt)
	assert.Equal(t, f.owner.ID, stored.AcceptedBy)
}

func TestPostgresInvite_AcceptVersusRevoke(t *testing.T) {
	for range 5 {
		f := newPGFixture(t)
		bob := f.user("bob-" + uuid.NewString()[:8] + "@example.com")
		c := f.invite(bob.Email, member.RoleMember)
		errs := race(
			func() error { _, err := f.svc.Accept(f.ctx, c.Invite.ID, bob.ID); return err },
			func() error { return f.svc.Revoke(f.ctx, member.Requester{UserID: f.owner.ID}, f.org.ID, c.Invite.ID) },
		)
		stored, err := f.store.ByID(f.ctx, c.Invite.ID)
		require.NoError(t, err)
		_, memberErr := f.members.Find(f.ctx, f.org.ID, bob.ID)
		if errs[0] == nil {
			assert.True(t, invite.IsNotFoundError(errs[1]), "revoke lost: %v", errs[1])
			assert.NotNil(t, stored.AcceptedAt)
			assert.Nil(t, stored.RevokedAt)
			assert.NoError(t, memberErr)
		} else {
			require.NoError(t, errs[1], "revoke won")
			assert.Equal(t, invite.ReasonRevoked, invite.ReasonOf(errs[0]))
			assert.Nil(t, stored.AcceptedAt)
			assert.NotNil(t, stored.RevokedAt)
			assert.True(t, member.IsNotFoundError(memberErr))
		}
	}
}

func TestPostgresInvite_ReinviteAfterExpiry(t *testing.T) {
	f := newPGFixture(t)
	first := f.invite("late@example.com", member.RoleMember)
	f.advance(invite.TTL + time.Hour)
	second := f.invite("late@example.com", member.RoleAdmin)
	assert.Equal(t, first.Invite.ID, second.Invite.ID, "updated in place")

	late := f.user("late@example.com")
	got, err := f.svc.Accept(f.ctx, second.Invite.ID, late.ID)
	require.NoError(t, err)
	assert.Equal(t, invite.Joined, got)

	third := f.invite("late@example.com", member.RoleMember)
	assert.NotEqual(t, first.Invite.ID, third.Invite.ID, "after an accept, a new row")
}

func TestPostgresInvite_ConcurrentCreatesShareOneRow(t *testing.T) {
	f := newPGFixture(t)
	ids := make([]uuid.UUID, 2)
	r := member.Requester{UserID: f.owner.ID}
	errs := race(
		func() error {
			c, err := f.svc.Create(f.ctx, r, f.org.ID, "twin@example.com", member.RoleMember)
			setID(ids, 0, c)
			return err
		},
		func() error {
			c, err := f.svc.Create(f.ctx, r, f.org.ID, "twin@example.com", member.RoleMember)
			setID(ids, 1, c)
			return err
		},
	)
	require.NoError(t, errors.Join(errs...))
	assert.Equal(t, ids[0], ids[1])
	pending, err := f.svc.ListPending(f.ctx, f.org.ID)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}

func setID(ids []uuid.UUID, i int, c *invite.Created) {
	if c != nil {
		ids[i] = c.Invite.ID
	}
}

func TestPostgresInvite_DraftsAndListing(t *testing.T) {
	f := newPGFixture(t)
	r := member.Requester{UserID: f.owner.ID}
	d, err := f.svc.Draft(f.ctx, r, f.org.ID, "dina@example.com")
	require.NoError(t, err)
	f.invite("bob@example.com", member.RoleAdmin)

	pending, err := f.svc.ListPending(f.ctx, f.org.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, f.owner.Email, pending[0].InviterName, "an empty name falls back to the email")
	drafts, err := f.svc.ListDrafts(f.ctx, f.org.ID)
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	assert.Equal(t, d.ID, drafts[0].ID)
	assert.Empty(t, drafts[0].TokenHash)

	c, err := f.svc.ConfirmDraft(f.ctx, r, f.org.ID, d.ID)
	require.NoError(t, err)
	got, err := f.svc.Lookup(f.ctx, c.Link[len(baseURL)+len("invite/"):])
	require.NoError(t, err)
	assert.Equal(t, d.ID, got.ID)
	byPublic, err := f.svc.ByPublicID(f.ctx, d.PublicID)
	require.NoError(t, err)
	assert.Equal(t, d.ID, byPublic.ID)

	require.NoError(t, f.svc.Revoke(f.ctx, r, f.org.ID, d.ID))
	pending, err = f.svc.ListPending(f.ctx, f.org.ID)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}

func TestPostgresInvite_LockNeedsATransaction(t *testing.T) {
	f := newPGFixture(t)
	_, err := f.store.Lock(f.ctx, uuid.New())
	assert.Error(t, err)
	_, err = f.store.Live(f.ctx, f.org.ID, "x@example.com")
	assert.Error(t, err)
	assert.Error(t, f.store.LockEmail(f.ctx, f.org.ID, "x@example.com"))
}

func TestPostgresInvite_StoreErrors(t *testing.T) {
	f := newPGFixture(t)
	assert.True(t, invite.IsNotFoundError(f.store.Update(f.ctx, &invite.Invite{ID: uuid.New(), Role: member.RoleMember})))
	assert.True(t, invite.IsNotFoundError(f.store.Revoke(f.ctx, uuid.New(), f.now())))
	_, err := f.store.MarkAccepted(f.ctx, uuid.New(), f.owner.ID, f.now())
	assert.True(t, invite.IsNotFoundError(err))
	closed, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, closed.Close())
	broken := invite.NewStore(db.Pool{W: closed, R: closed}, "public", "")
	_, err = broken.ByID(f.ctx, uuid.New())
	assert.False(t, invite.IsNotFoundError(err))
	assert.Error(t, err)
	_, err = broken.ListLive(f.ctx, f.org.ID)
	assert.Error(t, err)
	assert.Error(t, broken.Insert(f.ctx, &invite.Invite{ID: uuid.New()}))
	assert.Error(t, broken.Update(f.ctx, &invite.Invite{ID: uuid.New()}))
	_, err = broken.MarkAccepted(f.ctx, uuid.New(), f.owner.ID, f.now())
	assert.Error(t, err)
}

func TestPostgresInvite_JoinIsOneTransaction(t *testing.T) {
	f := newPGFixture(t)
	c := f.invite("new@example.com", member.RoleAdmin)
	id, err := f.svc.Join(f.ctx, c.Invite.ID, "New", "a sturdy passphrase", true)
	require.NoError(t, err)
	u, err := f.users.ByID(f.ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, u.TermsAcceptedAt)
	m, err := f.members.Find(f.ctx, f.org.ID, id)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, m.Role)

	late := f.invite("late@example.com", member.RoleMember)
	f.advance(invite.TTL)
	_, err = f.svc.Join(f.ctx, late.Invite.ID, "Late", "a sturdy passphrase", true)
	assert.Equal(t, invite.ReasonExpired, invite.ReasonOf(err))
	_, err = f.users.ByEmail(f.ctx, "late@example.com")
	assert.True(t, user.IsNotFoundError(err), "a refused join writes nothing")
}

func TestPostgresInvite_ConcurrentJoinsMakeOneUser(t *testing.T) {
	f := newPGFixture(t)
	c := f.invite("twin@example.com", member.RoleMember)
	errs := race(
		func() error { _, err := f.svc.Join(f.ctx, c.Invite.ID, "A", "a sturdy passphrase", true); return err },
		func() error { _, err := f.svc.Join(f.ctx, c.Invite.ID, "B", "a sturdy passphrase", true); return err },
	)
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case user.IsAlreadyExistsError(err), invite.ReasonOf(err) == invite.ReasonUsed:
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	assert.Equal(t, 1, ok)
	ms, err := f.members.List(f.ctx, f.org.ID)
	require.NoError(t, err)
	assert.Len(t, ms, 2)
}

func TestPostgresInvite_ReinviteVersusAccept(t *testing.T) {
	for range 8 {
		f := newPGFixture(t)
		bob := f.user("bob-" + uuid.NewString()[:8] + "@example.com")
		first := f.invite(bob.Email, member.RoleMember)
		invite.SetAfterLive(f.svc, func() { time.Sleep(150 * time.Millisecond) })
		var again *invite.Created
		errs := race(
			func() error { _, err := f.svc.Accept(f.ctx, first.Invite.ID, bob.ID); return err },
			func() (err error) {
				again, err = f.svc.Create(f.ctx, member.Requester{UserID: f.owner.ID}, f.org.ID, bob.Email, member.RoleAdmin)
				return err
			},
		)
		require.NoError(t, errors.Join(errs...))
		accepted, err := f.store.ByID(f.ctx, first.Invite.ID)
		require.NoError(t, err)
		require.NotNil(t, accepted.AcceptedAt)
		m, err := f.members.Find(f.ctx, f.org.ID, bob.ID)
		require.NoError(t, err)
		assert.Equal(t, m.Role, accepted.Role, "an accepted invite is never rewritten after the accept")
		if again.Invite.ID != first.Invite.ID {
			fresh, err := f.store.ByID(f.ctx, again.Invite.ID)
			require.NoError(t, err)
			assert.Nil(t, fresh.AcceptedAt, "the new invite is live")
		}
	}
}
