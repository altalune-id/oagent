package member_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/mailer"
)

func newService(store member.Store) *member.Service {
	return member.NewService(store, fakes.TxPool(), apperror.NewReporter(nil, false).Unexpected)
}

func isUnexpected(err error) bool {
	ae, ok := errors.AsType[*apperror.AppError](err)
	return ok && ae.Code() == apperror.CodeUnexpected
}

type orgFixture struct {
	t     *testing.T
	ctx   context.Context
	store *fakes.MemberStore
	svc   *member.Service
	org   uuid.UUID
}

func newOrgFixture(t *testing.T) *orgFixture {
	store := &fakes.MemberStore{}
	return &orgFixture{t: t, ctx: context.Background(), store: store, svc: newService(store), org: uuid.New()}
}

func (f *orgFixture) add(role member.Role) member.Requester {
	f.t.Helper()
	m, err := f.svc.Add(f.ctx, f.org, uuid.New(), role)
	require.NoError(f.t, err)
	return member.Requester{UserID: m.UserID}
}

func (f *orgFixture) role(userID uuid.UUID) member.Role {
	f.t.Helper()
	m, err := f.svc.Find(f.ctx, f.org, userID)
	require.NoError(f.t, err)
	return m.Role
}

func lastOwnerReason(err error) member.LastOwnerReason {
	lo, ok := errors.AsType[*member.LastOwnerError](err)
	if !ok {
		return ""
	}
	return lo.Reason
}

var operator = member.Requester{Operator: true}

func TestService_AddFindList(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	usr, orgA, orgB := uuid.New(), uuid.New(), uuid.New()

	m, err := svc.Add(ctx, orgA, usr, member.RoleOwner)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	_, err = svc.Add(ctx, orgB, usr, member.RoleMember)
	require.NoError(t, err)

	got, err := svc.Find(ctx, orgA, usr)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)

	list, err := svc.ListByUser(ctx, usr)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestService_ExpectedErrors(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	org, usr := uuid.New(), uuid.New()

	_, err := svc.Add(ctx, org, usr, "root")
	assert.True(t, member.IsInvalidRoleError(err))
	_, err = svc.Add(ctx, org, usr, member.RoleAdmin)
	require.NoError(t, err)
	_, err = svc.Add(ctx, org, usr, member.RoleMember)
	assert.True(t, member.IsAlreadyMemberError(err))
	_, err = svc.Find(ctx, org, uuid.New())
	assert.True(t, member.IsNotFoundError(err))
}

func TestService_StoreFailureIsReported(t *testing.T) {
	svc := newService(&fakes.MemberStore{Err: errors.New("db down")})
	ctx := context.Background()
	_, err := svc.Add(ctx, uuid.New(), uuid.New(), member.RoleOwner)
	assert.True(t, isUnexpected(err), "got %v", err)
	_, err = svc.Find(ctx, uuid.New(), uuid.New())
	assert.True(t, isUnexpected(err))
	_, err = svc.ListByUser(ctx, uuid.New())
	assert.True(t, isUnexpected(err))
	_, err = svc.CountOwners(ctx, uuid.New())
	assert.True(t, isUnexpected(err))
	_, err = svc.List(ctx, uuid.New())
	assert.True(t, isUnexpected(err))
	_, err = svc.SetRole(ctx, operator, uuid.New(), uuid.New(), member.RoleAdmin)
	assert.True(t, isUnexpected(err), "got %v", err)
	assert.True(t, isUnexpected(svc.Remove(ctx, operator, uuid.New(), uuid.New())))
	assert.True(t, isUnexpected(svc.Leave(ctx, uuid.New(), uuid.New())))
	_, err = svc.AddExisting(ctx, operator, uuid.New(), uuid.New(), member.RoleMember)
	assert.True(t, isUnexpected(err))
}

func TestService_TxBeginFailureIsReported(t *testing.T) {
	svc := member.NewService(&fakes.MemberStore{}, db.Pool{}, apperror.NewReporter(nil, false).Unexpected)
	assert.True(t, isUnexpected(svc.Remove(context.Background(), operator, uuid.New(), uuid.New())))
}

func TestService_MissingMembership(t *testing.T) {
	f := newOrgFixture(t)
	f.add(member.RoleOwner)
	_, err := f.svc.SetRole(f.ctx, operator, f.org, uuid.New(), member.RoleAdmin)
	assert.True(t, member.IsNotFoundError(err), "set role: %v", err)
	assert.True(t, member.IsNotFoundError(f.svc.Remove(f.ctx, operator, f.org, uuid.New())))
	assert.True(t, member.IsNotFoundError(f.svc.Leave(f.ctx, f.org, uuid.New())), "leave as a non-member")
}

func TestService_LastOwner(t *testing.T) {
	f := newOrgFixture(t)
	owner := f.add(member.RoleOwner)
	f.add(member.RoleAdmin)

	_, err := f.svc.SetRole(f.ctx, operator, f.org, owner.UserID, member.RoleAdmin)
	assert.Equal(t, member.LastOwnerDemote, lastOwnerReason(err), "got %v", err)
	assert.Equal(t, member.LastOwnerRemove, lastOwnerReason(f.svc.Remove(f.ctx, operator, f.org, owner.UserID)))
	assert.Equal(t, member.LastOwnerLeave, lastOwnerReason(f.svc.Leave(f.ctx, f.org, owner.UserID)))
	assert.Equal(t, member.RoleOwner, f.role(owner.UserID), "nothing changed")

	m, err := f.svc.SetRole(f.ctx, operator, f.org, owner.UserID, member.RoleOwner)
	require.NoError(t, err, "keeping the last owner an owner is fine")
	assert.Equal(t, member.RoleOwner, m.Role)
}

func TestService_TwoOwners(t *testing.T) {
	f := newOrgFixture(t)
	a, b := f.add(member.RoleOwner), f.add(member.RoleOwner)

	m, err := f.svc.SetRole(f.ctx, a, f.org, b.UserID, member.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, m.Role)
	assert.Equal(t, member.RoleAdmin, f.role(b.UserID))
	n, err := f.svc.CountOwners(f.ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = f.svc.SetRole(f.ctx, a, f.org, b.UserID, member.RoleOwner)
	require.NoError(t, err)
	require.NoError(t, f.svc.Leave(f.ctx, f.org, b.UserID), "one of two owners may leave")
	_, err = f.svc.Find(f.ctx, f.org, b.UserID)
	assert.True(t, member.IsNotFoundError(err))
}

func TestService_RulesApply(t *testing.T) {
	f := newOrgFixture(t)
	owner := f.add(member.RoleOwner)
	admin := f.add(member.RoleAdmin)
	plain := f.add(member.RoleMember)

	assert.True(t, member.IsRoleNotAllowedError(f.svc.Remove(f.ctx, admin, f.org, owner.UserID)))
	_, err := f.svc.SetRole(f.ctx, admin, f.org, plain.UserID, member.RoleOwner)
	assert.True(t, member.IsRoleNotAllowedError(err))
	assert.True(t, member.IsSelfChangeError(f.svc.Remove(f.ctx, admin, f.org, admin.UserID)))
	_, err = f.svc.SetRole(f.ctx, plain, f.org, admin.UserID, member.RoleMember)
	assert.True(t, authz.IsForbiddenError(err), "got %v", err)
	_, err = f.svc.SetRole(f.ctx, owner, f.org, plain.UserID, "")
	assert.True(t, member.IsInvalidRoleError(err), "set role needs a role")

	_, err = f.svc.SetRole(f.ctx, admin, f.org, plain.UserID, member.RoleAdmin)
	require.NoError(t, err)
	require.NoError(t, f.svc.Remove(f.ctx, admin, f.org, plain.UserID))
	_, err = f.svc.Find(f.ctx, f.org, plain.UserID)
	assert.True(t, member.IsNotFoundError(err))
	require.NoError(t, f.svc.Leave(f.ctx, f.org, admin.UserID), "a non-owner may always leave")
}

func TestService_AddExisting(t *testing.T) {
	f := newOrgFixture(t)
	owner := f.add(member.RoleOwner)
	admin := f.add(member.RoleAdmin)
	plain := f.add(member.RoleMember)

	_, err := f.svc.AddExisting(f.ctx, operator, f.org, plain.UserID, member.RoleOwner)
	assert.True(t, member.IsAlreadyMemberError(err), "got %v", err)
	assert.Equal(t, member.RoleMember, f.role(plain.UserID), "the role is never changed")

	_, err = f.svc.AddExisting(f.ctx, admin, f.org, uuid.New(), member.RoleOwner)
	assert.True(t, member.IsRoleNotAllowedError(err))
	_, err = f.svc.AddExisting(f.ctx, plain, f.org, uuid.New(), member.RoleMember)
	assert.True(t, authz.IsForbiddenError(err))
	_, err = f.svc.AddExisting(f.ctx, operator, f.org, uuid.New(), "")
	assert.True(t, member.IsInvalidRoleError(err))

	m, err := f.svc.AddExisting(f.ctx, admin, f.org, uuid.New(), member.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, f.role(m.UserID))
	m, err = f.svc.AddExisting(f.ctx, owner, f.org, uuid.New(), member.RoleOwner)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
}

func TestService_List(t *testing.T) {
	f := newOrgFixture(t)
	rows := []struct {
		role  member.Role
		email string
	}{
		{member.RoleMember, "a@example.com"},
		{member.RoleOwner, "z@example.com"},
		{member.RoleAdmin, "c@example.com"},
		{member.RoleOwner, "b@example.com"},
	}
	for _, r := range rows {
		a := f.add(r.role)
		f.store.SetContact(a.UserID, fakes.MemberContact{Email: r.email, Name: "N " + r.email})
	}
	_, err := f.svc.Add(f.ctx, uuid.New(), uuid.New(), member.RoleOwner)
	require.NoError(t, err)

	got, err := f.svc.List(f.ctx, f.org)
	require.NoError(t, err)
	emails := make([]string, 0, len(got))
	for _, m := range got {
		emails = append(emails, m.Email)
		assert.Equal(t, "N "+m.Email, m.Name)
	}
	assert.Equal(t, []string{"b@example.com", "z@example.com", "c@example.com", "a@example.com"}, emails)
}

func TestService_CountOwners(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	orgID := uuid.New()

	n, err := svc.CountOwners(ctx, orgID)
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = svc.Add(ctx, orgID, uuid.New(), member.RoleOwner)
	require.NoError(t, err)
	_, err = svc.Add(ctx, orgID, uuid.New(), member.RoleAdmin)
	require.NoError(t, err)
	n, err = svc.CountOwners(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

type effectCalls struct {
	mu            sync.Mutex
	sessionsEnded []uuid.UUID
	keysRevoked   [][2]uuid.UUID
	mail          []mailer.Message
	failSessions  error
	failKeys      error
	failMail      error
	failContact   error
	failOrg       error
	orgName       string
}

func (e *effectCalls) DeleteByUser(_ context.Context, userID uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionsEnded = append(e.sessionsEnded, userID)
	return e.failSessions
}

func (e *effectCalls) RevokePersonalOf(_ context.Context, orgID, userID uuid.UUID) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.keysRevoked = append(e.keysRevoked, [2]uuid.UUID{orgID, userID})
	return 1, e.failKeys
}

func (e *effectCalls) Contact(_ context.Context, userID uuid.UUID) (string, string, error) {
	return userID.String() + "@example.com", "Person", e.failContact
}

func (e *effectCalls) Name(context.Context, uuid.UUID) (string, error) {
	return e.orgName, e.failOrg
}

func (e *effectCalls) Enqueue(_ context.Context, m mailer.Message) (uuid.UUID, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failMail != nil {
		return uuid.Nil, e.failMail
	}
	e.mail = append(e.mail, m)
	return uuid.New(), nil
}

func (e *effectCalls) effects() member.Effects {
	return member.Effects{
		Sessions: e, Keys: e, People: e, Orgs: e, Mail: e,
		From: "team@example.com", BaseURL: "https://oagent.example.com/base",
	}
}

func newEffectsFixture(t *testing.T) (*orgFixture, *effectCalls) {
	f := newOrgFixture(t)
	e := &effectCalls{orgName: "Acme Corp"}
	f.svc.WithEffects(e.effects())
	return f, e
}

func TestService_RemoveEndsAccessAndNotifies(t *testing.T) {
	f, e := newEffectsFixture(t)
	owner := f.add(member.RoleOwner)
	gone := f.add(member.RoleMember)

	require.NoError(t, f.svc.Remove(f.ctx, owner, f.org, gone.UserID))

	assert.Equal(t, []uuid.UUID{gone.UserID}, e.sessionsEnded)
	assert.Equal(t, [][2]uuid.UUID{{f.org, gone.UserID}}, e.keysRevoked)
	require.Len(t, e.mail, 1)
	m := e.mail[0]
	assert.Equal(t, gone.UserID.String()+"@example.com", m.To)
	assert.Equal(t, "team@example.com", m.From)
	assert.Equal(t, "You were removed from Acme Corp", m.Subject)
	for _, body := range []string{m.TextBody, m.HTMLBody} {
		assert.Contains(t, body, "Acme Corp")
		assert.NotContains(t, body, "http", "no link back to the org")
	}
	assert.NoError(t, m.Validate())
}

func TestService_RoleChangeNotifiesAndDemotionEndsSessions(t *testing.T) {
	cases := []struct {
		name      string
		from, to  member.Role
		subject   string
		signedOut bool
	}{
		{"demotion", member.RoleAdmin, member.RoleMember, "Your role in Acme Corp is now Member", true},
		{"owner to member", member.RoleOwner, member.RoleMember, "Your role in Acme Corp is now Member", true},
		{"promotion", member.RoleMember, member.RoleAdmin, "Your role in Acme Corp is now Admin", false},
		{"to owner", member.RoleAdmin, member.RoleOwner, "Your role in Acme Corp is now Owner", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, e := newEffectsFixture(t)
			owner := f.add(member.RoleOwner)
			target := f.add(tc.from)

			_, err := f.svc.SetRole(f.ctx, owner, f.org, target.UserID, tc.to)
			require.NoError(t, err)

			if tc.signedOut {
				assert.Equal(t, []uuid.UUID{target.UserID}, e.sessionsEnded, "a demotion signs the person out")
			} else {
				assert.Empty(t, e.sessionsEnded, "a promotion applies on the next request")
			}
			assert.Empty(t, e.keysRevoked, "a role change keeps personal keys")
			require.Len(t, e.mail, 1)
			m := e.mail[0]
			assert.Equal(t, tc.subject, m.Subject)
			assert.Contains(t, m.TextBody, "https://oagent.example.com/base/console/")
			assert.Contains(t, m.HTMLBody, `href="https://oagent.example.com/base/console/"`)
		})
	}
}

func TestService_SameRoleHasNoEffects(t *testing.T) {
	f, e := newEffectsFixture(t)
	owner := f.add(member.RoleOwner)
	admin := f.add(member.RoleAdmin)

	_, err := f.svc.SetRole(f.ctx, owner, f.org, admin.UserID, member.RoleAdmin)
	require.NoError(t, err)
	assert.Empty(t, e.sessionsEnded)
	assert.Empty(t, e.mail)
}

func TestService_LeaveRevokesPersonalKeysOnly(t *testing.T) {
	f, e := newEffectsFixture(t)
	f.add(member.RoleOwner)
	plain := f.add(member.RoleMember)

	require.NoError(t, f.svc.Leave(f.ctx, f.org, plain.UserID))
	assert.Equal(t, [][2]uuid.UUID{{f.org, plain.UserID}}, e.keysRevoked)
	assert.Empty(t, e.sessionsEnded, "leaving keeps the person signed in elsewhere")
	assert.Empty(t, e.mail)
}

func TestService_OrgNameIsOneLineInTheSubject(t *testing.T) {
	f, e := newEffectsFixture(t)
	e.orgName = "Acme\r\nBcc: x@example.com"
	owner := f.add(member.RoleOwner)
	gone := f.add(member.RoleMember)

	require.NoError(t, f.svc.Remove(f.ctx, owner, f.org, gone.UserID))
	require.Len(t, e.mail, 1)
	assert.Equal(t, "You were removed from Acme Bcc: x@example.com", e.mail[0].Subject)
	assert.NoError(t, e.mail[0].Validate())
}

func TestService_FailedEffectFailsTheChange(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(*effectCalls){
		"sessions": func(e *effectCalls) { e.failSessions = boom },
		"keys":     func(e *effectCalls) { e.failKeys = boom },
		"contact":  func(e *effectCalls) { e.failContact = boom },
		"org name": func(e *effectCalls) { e.failOrg = boom },
		"mail":     func(e *effectCalls) { e.failMail = boom },
	}
	for name, fail := range cases {
		t.Run(name, func(t *testing.T) {
			f, e := newEffectsFixture(t)
			fail(e)
			owner := f.add(member.RoleOwner)

			err := f.svc.Remove(f.ctx, owner, f.org, f.add(member.RoleAdmin).UserID)
			assert.True(t, isUnexpected(err), "remove: %v", err)
			if name == "keys" {
				err = f.svc.Leave(f.ctx, f.org, f.add(member.RoleMember).UserID)
				assert.True(t, isUnexpected(err), "leave: %v", err)
				return
			}
			_, err = f.svc.SetRole(f.ctx, owner, f.org, f.add(member.RoleAdmin).UserID, member.RoleMember)
			assert.True(t, isUnexpected(err), "set role: %v", err)
		})
	}
}

func TestService_NobodyIsForbidden(t *testing.T) {
	f := newOrgFixture(t)
	f.add(member.RoleOwner)
	target := f.add(member.RoleMember)
	_, err := f.svc.SetRole(f.ctx, member.Requester{}, f.org, target.UserID, member.RoleAdmin)
	assert.True(t, authz.IsForbiddenError(err), "got %v", err)
	assert.True(t, authz.IsForbiddenError(f.svc.Remove(f.ctx, member.Requester{}, f.org, target.UserID)))
	_, err = f.svc.AddExisting(f.ctx, member.Requester{}, f.org, uuid.New(), member.RoleMember)
	assert.True(t, authz.IsForbiddenError(err), "got %v", err)
}
