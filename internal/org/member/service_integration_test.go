//go:build integration

package member_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org/member"
)

func (f *memberFixture) service() *member.Service {
	return member.NewService(f.store, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected)
}

func TestService_OwnersRemovingEachOther(t *testing.T) {
	for name, tc := range map[string]struct {
		byOperator bool
		refused    func(t *testing.T, err error)
	}{
		"each owner removes the other": {
			refused: func(t *testing.T, err error) {
				assert.True(t, authz.IsForbiddenError(err), "the loser is no longer a member: %v", err)
			},
		},
		"the operator removes both owners": {
			byOperator: true,
			refused: func(t *testing.T, err error) {
				lo, ok := errors.AsType[*member.LastOwnerError](err)
				if assert.True(t, ok, "the other is refused as the last owner: %v", err) {
					assert.Equal(t, member.LastOwnerRemove, lo.Reason)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for range 3 {
				f := newMemberFixture(t)
				svc := f.service()
				member.SetAfterLock(svc, func() { time.Sleep(150 * time.Millisecond) })
				o := f.org()
				a, b := f.user(), f.user()
				f.add(o.ID, a.ID, member.RoleOwner)
				f.add(o.ID, b.ID, member.RoleOwner)
				byA, byB := member.Requester{UserID: a.ID}, member.Requester{UserID: b.ID}
				if tc.byOperator {
					byA, byB = member.Requester{Operator: true}, member.Requester{Operator: true}
				}

				var ready, done sync.WaitGroup
				start := make(chan struct{})
				errs := make([]error, 2)
				for i, call := range []func() error{
					func() error { return svc.Remove(f.ctx, byA, o.ID, b.ID) },
					func() error { return svc.Remove(f.ctx, byB, o.ID, a.ID) },
				} {
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

				failed := errors.Join(errs...)
				assert.Equal(t, 1, countNil(errs), "exactly one removal succeeds: %v", failed)
				tc.refused(t, failed)
				n, err := f.store.CountOwners(f.ctx, o.ID)
				require.NoError(t, err)
				assert.Equal(t, 1, n, "one owner remains")
			}
		})
	}
}

// Review Focus: the actor's role is read under the owner lock, so a demoted owner can't use the role they just lost.
func TestService_ActorRoleIsReadUnderTheLock(t *testing.T) {
	f := newMemberFixture(t)
	svc := f.service()
	o := f.org()
	a, b, c := f.user(), f.user(), f.user()
	f.add(o.ID, a.ID, member.RoleOwner)
	f.add(o.ID, b.ID, member.RoleOwner)
	f.add(o.ID, c.ID, member.RoleMember)

	locked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	member.SetAfterLock(svc, func() {
		once.Do(func() {
			close(locked)
			<-release
		})
	})
	demoted, promoted := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := svc.SetRole(f.ctx, member.Requester{UserID: a.ID}, o.ID, b.ID, member.RoleAdmin)
		demoted <- err
	}()
	<-locked
	go func() {
		_, err := svc.SetRole(f.ctx, member.Requester{UserID: b.ID}, o.ID, c.ID, member.RoleOwner)
		promoted <- err
	}()
	f.waitForLockWaiter()
	close(release)

	require.NoError(t, <-demoted)
	err := <-promoted
	assert.True(t, member.IsRoleNotAllowedError(err), "B is an admin once A commits: %v", err)
	m, err := f.store.Find(f.ctx, o.ID, c.ID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleMember, m.Role)
}

func (f *memberFixture) waitForLockWaiter() {
	f.t.Helper()
	require.Eventually(f.t, func() bool {
		var n int
		err := f.pool.W.QueryRowContext(f.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND datname = current_database() AND query LIKE '%' || $1 || '%'`, f.schema).Scan(&n)
		return err == nil && n > 0
	}, 10*time.Second, 10*time.Millisecond, "the second request waits on the owner lock")
}

func countNil(errs []error) int {
	n := 0
	for _, err := range errs {
		if err == nil {
			n++
		}
	}
	return n
}

func TestService_LastOwnerOnPostgres(t *testing.T) {
	f := newMemberFixture(t)
	svc := f.service()
	o := f.org()
	owner, admin := f.user(), f.user()
	f.add(o.ID, owner.ID, member.RoleOwner)
	f.add(o.ID, admin.ID, member.RoleAdmin)
	operator := member.Requester{Operator: true}

	_, err := svc.SetRole(f.ctx, operator, o.ID, owner.ID, member.RoleMember)
	assert.True(t, member.IsLastOwnerError(err), "got %v", err)
	assert.True(t, member.IsLastOwnerError(svc.Leave(f.ctx, o.ID, owner.ID)))

	_, err = svc.SetRole(f.ctx, operator, o.ID, admin.ID, member.RoleOwner)
	require.NoError(t, err)
	require.NoError(t, svc.Leave(f.ctx, o.ID, owner.ID))
	n, err := f.store.CountOwners(f.ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := svc.List(f.ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, admin.ID, got[0].UserID)
	assert.Equal(t, admin.Email, got[0].Email)

	added, err := svc.AddExisting(f.ctx, operator, o.ID, owner.ID, member.RoleMember)
	require.NoError(t, err)
	assert.Equal(t, member.RoleMember, added.Role)
	_, err = svc.AddExisting(f.ctx, operator, o.ID, owner.ID, member.RoleAdmin)
	assert.True(t, member.IsAlreadyMemberError(err), "got %v", err)
}

func TestService_FailedEffectRollsBack(t *testing.T) {
	f := newMemberFixture(t)
	svc := f.service()
	e := &effectCalls{orgName: "Acme Corp", failMail: errors.New("queue down")}
	svc.WithEffects(e.effects())
	o := f.org()
	owner, admin := f.user(), f.user()
	f.add(o.ID, owner.ID, member.RoleOwner)
	f.add(o.ID, admin.ID, member.RoleAdmin)
	actor := member.Requester{UserID: owner.ID}

	assert.Error(t, svc.Remove(f.ctx, actor, o.ID, admin.ID))
	_, err := svc.SetRole(f.ctx, actor, o.ID, admin.ID, member.RoleMember)
	assert.Error(t, err)

	m, err := f.store.Find(f.ctx, o.ID, admin.ID)
	require.NoError(t, err, "the removal rolled back")
	assert.Equal(t, member.RoleAdmin, m.Role, "the role change rolled back")
	assert.Len(t, e.sessionsEnded, 2, "effects ran inside the transaction")
}
