package invite_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/password"
)

const baseURL = "https://oagent.example.com/"

type fixture struct {
	t       *testing.T
	ctx     context.Context
	store   *memStore
	members *member.Service
	mail    *mailBox
	events  *eventLog
	people  people
	users   *user.Service
	svc     *invite.Service
	org     uuid.UUID
	clock   time.Time
	owner   member.Requester
	admin   member.Requester
	plain   member.Requester
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{
		t: t, ctx: context.Background(), store: newMemStore(), mail: &mailBox{}, events: &eventLog{}, people: people{},
		org: uuid.New(), clock: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC),
	}
	f.members = member.NewService(&fakes.MemberStore{}, fakes.Transactor{}, apperror.NewReporter(nil, false).Unexpected)
	f.users = user.NewService(fakes.NewUserStore(), apperror.NewReporter(nil, false).Unexpected)
	f.svc = invite.NewService(invite.Deps{
		Store: f.store, Transactor: fakes.Transactor{}, Members: f.members, Mail: f.mail, Users: f.users, Terms: legal.NewService(f.users), Events: f.events,
		Orgs: orgNames{f.org: "Acme  Corp"}, People: directory{people: f.people, users: f.users}, From: "oagent@example.com", BaseURL: baseURL,
		Unexpected: apperror.NewReporter(nil, false).Unexpected,
	}).WithClock(func() time.Time { return f.clock })
	f.owner = f.person(member.RoleOwner, "olga@example.com", "Olga")
	f.admin = f.person(member.RoleAdmin, "adi@example.com", "")
	f.plain = f.person(member.RoleMember, "mia@example.com", "Mia")
	return f
}

func (f *fixture) person(role member.Role, email, name string) member.Requester {
	f.t.Helper()
	id := uuid.New()
	_, err := f.members.Add(f.ctx, f.org, id, role)
	require.NoError(f.t, err)
	f.people[id] = contact{email: email, name: name}
	return member.Requester{UserID: id}
}

func (f *fixture) newcomer(email string) uuid.UUID {
	id := uuid.New()
	f.people[id] = contact{email: email}
	return id
}

func (f *fixture) create(email string, role member.Role) *invite.Created {
	f.t.Helper()
	c, err := f.svc.Create(f.ctx, f.owner, f.org, email, role)
	require.NoError(f.t, err)
	return c
}

func rawToken(t *testing.T, link string) string {
	t.Helper()
	raw, ok := strings.CutPrefix(link, "https://oagent.example.com/invite/")
	require.True(t, ok, "link %q", link)
	return raw
}

func isUnexpected(err error) bool {
	ae, ok := errors.AsType[*apperror.AppError](err)
	return ok && ae.Code() == apperror.CodeUnexpected
}

func TestService_CreateNormalizesAndValidates(t *testing.T) {
	f := newFixture(t)
	c := f.create("  Bob@X.io ", member.RoleMember)
	assert.Equal(t, "bob@x.io", c.Invite.Email)

	_, err := f.svc.Create(f.ctx, f.owner, f.org, "not an email", member.RoleMember)
	assert.True(t, user.IsInvalidEmailError(err), "got %v", err)
	_, err = f.svc.Create(f.ctx, f.owner, f.org, "bob@x.io", member.Role("root"))
	assert.True(t, member.IsInvalidRoleError(err), "got %v", err)
}

func TestService_CreateQueuesTheLinkAndStoresOnlyTheHash(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleAdmin)
	raw := rawToken(t, c.Link)

	require.Equal(t, 1, f.mail.count())
	m := f.mail.sent[0]
	assert.Equal(t, "bob@x.io", m.To)
	assert.Equal(t, "oagent@example.com", m.From)
	assert.Equal(t, "Olga invited you to Acme Corp on oagent", m.Subject)
	assert.Contains(t, m.TextBody, c.Link)
	assert.Contains(t, m.HTMLBody, c.Link)
	assert.Contains(t, m.TextBody, "as Admin")
	assert.Contains(t, m.TextBody, "20 October 2026")

	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, stored.TokenHash)
	assert.NotContains(t, stored.TokenHash, raw)
	assert.Equal(t, f.clock.Add(invite.TTL), stored.ExpiresAt)
	assert.Equal(t, f.owner.UserID, stored.InvitedBy)
	assert.Equal(t, invite.StatusPending, stored.Status(f.clock))
	assert.Len(t, stored.PublicID, 21)
}

func TestService_CreateNamesTheInviter(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(f.ctx, f.admin, f.org, "bob@x.io", member.RoleMember)
	require.NoError(t, err)
	_, err = f.svc.Create(f.ctx, member.Requester{Operator: true}, f.org, "eve@x.io", member.RoleMember)
	require.NoError(t, err)
	require.Equal(t, 2, f.mail.count())
	assert.True(t, strings.HasPrefix(f.mail.sent[0].Subject, "adi@example.com invited you"), "an empty name falls back to the email")
	assert.True(t, strings.HasPrefix(f.mail.sent[1].Subject, "Someone invited you"), "an operator has no name")
}

func TestService_CreateAgainReplaces(t *testing.T) {
	f := newFixture(t)
	first := f.create("bob@x.io", member.RoleMember)
	f.clock = f.clock.Add(8 * 24 * time.Hour)
	second := f.create("bob@x.io", member.RoleAdmin)

	assert.Equal(t, first.Invite.ID, second.Invite.ID, "the same row")
	assert.NotEqual(t, first.Link, second.Link)
	assert.NotEqual(t, first.Invite.TokenHash, second.Invite.TokenHash)
	assert.Equal(t, f.clock.Add(invite.TTL), second.Invite.ExpiresAt)
	assert.Equal(t, member.RoleAdmin, second.Invite.Role)

	_, err := f.svc.Lookup(f.ctx, rawToken(t, first.Link))
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err), "the old link stops working")
	got, err := f.svc.Lookup(f.ctx, rawToken(t, second.Link))
	require.NoError(t, err)
	assert.Equal(t, second.Invite.ID, got.ID)
}

func TestService_RoleRules(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(f.ctx, f.admin, f.org, "bob@x.io", member.RoleOwner)
	assert.True(t, member.IsRoleNotAllowedError(err), "an admin can't invite an owner: %v", err)
	_, err = f.svc.Create(f.ctx, f.plain, f.org, "bob@x.io", member.RoleMember)
	assert.True(t, authz.IsForbiddenError(err), "a member can't invite: %v", err)
	_, err = f.svc.Create(f.ctx, member.Requester{}, f.org, "bob@x.io", member.RoleMember)
	assert.True(t, authz.IsForbiddenError(err), "nobody can't invite: %v", err)

	owned := f.create("own@x.io", member.RoleOwner)
	_, err = f.svc.Create(f.ctx, f.admin, f.org, "own@x.io", member.RoleMember)
	assert.True(t, member.IsRoleNotAllowedError(err), "an admin can't replace an owner invite: %v", err)
	_, err = f.svc.Resend(f.ctx, f.admin, f.org, owned.Invite.ID)
	assert.True(t, member.IsRoleNotAllowedError(err), "an admin can't resend an owner invite: %v", err)
	assert.True(t, member.IsRoleNotAllowedError(f.svc.Revoke(f.ctx, f.admin, f.org, owned.Invite.ID)))
	assert.Equal(t, 1, f.mail.count())
}

func TestService_ResendAndRevoke(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	f.clock = f.clock.Add(time.Hour)

	again, err := f.svc.Resend(f.ctx, f.admin, f.org, c.Invite.ID)
	require.NoError(t, err)
	assert.Equal(t, c.Invite.ID, again.Invite.ID)
	assert.NotEqual(t, c.Link, again.Link)
	assert.Equal(t, 2, f.mail.count())
	_, err = f.svc.Lookup(f.ctx, rawToken(t, c.Link))
	assert.True(t, invite.IsNotUsableError(err), "the old link stops working")

	require.NoError(t, f.svc.Revoke(f.ctx, f.admin, f.org, c.Invite.ID))
	_, err = f.svc.Lookup(f.ctx, rawToken(t, again.Link))
	assert.Equal(t, invite.ReasonRevoked, invite.ReasonOf(err))

	_, err = f.svc.Resend(f.ctx, f.admin, f.org, c.Invite.ID)
	assert.True(t, invite.IsNotFoundError(err), "a revoked invite is gone: %v", err)
	assert.True(t, invite.IsNotFoundError(f.svc.Revoke(f.ctx, f.admin, f.org, c.Invite.ID)))
	_, err = f.svc.Resend(f.ctx, f.admin, uuid.New(), c.Invite.ID)
	assert.True(t, invite.IsNotFoundError(err) || authz.IsForbiddenError(err), "another org never finds it: %v", err)
	_, err = f.svc.Resend(f.ctx, f.admin, f.org, uuid.New())
	assert.True(t, invite.IsNotFoundError(err), "an unknown id: %v", err)
}

func TestService_InviteInAnotherOrgIsNotFound(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	other := uuid.New()
	_, err := f.members.Add(f.ctx, other, f.owner.UserID, member.RoleOwner)
	require.NoError(t, err)
	_, err = f.svc.Resend(f.ctx, f.owner, other, c.Invite.ID)
	assert.True(t, invite.IsNotFoundError(err), "got %v", err)
}

func TestService_Lookup(t *testing.T) {
	f := newFixture(t)
	live := f.create("live@x.io", member.RoleMember)
	got, err := f.svc.Lookup(f.ctx, rawToken(t, live.Link))
	require.NoError(t, err)
	assert.Equal(t, live.Invite.ID, got.ID)

	_, err = f.svc.Lookup(f.ctx, "unknown-token")
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err))
	_, err = f.svc.Lookup(f.ctx, "")
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err))

	used := f.create("used@x.io", member.RoleMember)
	_, err = f.svc.Accept(f.ctx, used.Invite.ID, f.newcomer("used@x.io"))
	require.NoError(t, err)
	_, err = f.svc.Lookup(f.ctx, rawToken(t, used.Link))
	assert.Equal(t, invite.ReasonUsed, invite.ReasonOf(err))

	revoked := f.create("revoked@x.io", member.RoleMember)
	require.NoError(t, f.svc.Revoke(f.ctx, f.owner, f.org, revoked.Invite.ID))
	_, err = f.svc.Lookup(f.ctx, rawToken(t, revoked.Link))
	assert.Equal(t, invite.ReasonRevoked, invite.ReasonOf(err))

	f.clock = f.clock.Add(invite.TTL)
	_, err = f.svc.Lookup(f.ctx, rawToken(t, live.Link))
	assert.Equal(t, invite.ReasonExpired, invite.ReasonOf(err), "expiry is exclusive")

	f.store.Err = errors.New("db down")
	_, err = f.svc.Lookup(f.ctx, "x")
	assert.True(t, isUnexpected(err))
}

func TestService_Accept(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleAdmin)
	bob := f.newcomer("bob@x.io")

	got, err := f.svc.Accept(f.ctx, c.Invite.ID, bob)
	require.NoError(t, err)
	assert.Equal(t, invite.Joined, got)
	m, err := f.members.Find(f.ctx, f.org, bob)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, m.Role)

	_, err = f.svc.Accept(f.ctx, c.Invite.ID, bob)
	assert.Equal(t, invite.ReasonUsed, invite.ReasonOf(err), "a second accept: %v", err)

	other := f.create("carol@x.io", member.RoleMember)
	_, err = f.svc.Accept(f.ctx, other.Invite.ID, bob)
	assert.True(t, invite.IsEmailMismatchError(err), "got %v", err)

	_, err = f.svc.Accept(f.ctx, uuid.New(), bob)
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err))
}

func TestService_AcceptByAnOwnerKeepsTheRole(t *testing.T) {
	f := newFixture(t)
	c := f.create("olga@example.com", member.RoleMember)
	got, err := f.svc.Accept(f.ctx, c.Invite.ID, f.owner.UserID)
	require.NoError(t, err)
	assert.Equal(t, invite.AlreadyMember, got)
	m, err := f.members.Find(f.ctx, f.org, f.owner.UserID)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.NotNil(t, stored.AcceptedAt, "the invite is used")
}

func TestService_AcceptAfterExpiry(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	f.clock = f.clock.Add(invite.TTL + time.Second)
	_, err := f.svc.Accept(f.ctx, c.Invite.ID, f.newcomer("bob@x.io"))
	assert.Equal(t, invite.ReasonExpired, invite.ReasonOf(err))
}

func TestService_Drafts(t *testing.T) {
	f := newFixture(t)
	d, err := f.svc.Draft(f.ctx, f.admin, f.org, " Dina@X.io")
	require.NoError(t, err)
	assert.True(t, d.Draft)
	assert.Empty(t, d.TokenHash, "a draft has no token")
	assert.Equal(t, member.RoleMember, d.Role)
	assert.Equal(t, "dina@x.io", d.Email)
	assert.Equal(t, f.clock.Add(invite.DraftTTL), d.ExpiresAt)
	assert.Zero(t, f.mail.count(), "a draft sends no email")
	assert.Equal(t, invite.StatusDraft, d.Status(f.clock))

	_, err = f.svc.Accept(f.ctx, d.ID, f.newcomer("dina@x.io"))
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err), "a draft can't be accepted")

	pending, err := f.svc.ListPending(f.ctx, f.org)
	require.NoError(t, err)
	assert.Empty(t, pending)
	drafts, err := f.svc.ListDrafts(f.ctx, f.org)
	require.NoError(t, err)
	require.Len(t, drafts, 1)

	_, err = f.svc.ConfirmDraft(f.ctx, f.plain, f.org, d.ID)
	assert.True(t, authz.IsForbiddenError(err), "a non-manager can't confirm: %v", err)

	c, err := f.svc.ConfirmDraft(f.ctx, f.owner, f.org, d.ID)
	require.NoError(t, err)
	assert.False(t, c.Invite.Draft)
	assert.Equal(t, d.ID, c.Invite.ID)
	assert.Equal(t, f.clock.Add(invite.TTL), c.Invite.ExpiresAt)
	assert.Equal(t, 1, f.mail.count())
	_, err = f.svc.Lookup(f.ctx, rawToken(t, c.Link))
	require.NoError(t, err)

	_, err = f.svc.ConfirmDraft(f.ctx, f.owner, f.org, d.ID)
	assert.True(t, invite.IsNotFoundError(err), "a confirmed draft is no longer a draft: %v", err)
	_, err = f.svc.Draft(f.ctx, f.admin, f.org, "dina@x.io")
	assert.True(t, invite.IsPendingError(err), "a draft never replaces a pending invite: %v", err)
}

func TestService_DraftExpires(t *testing.T) {
	f := newFixture(t)
	d, err := f.svc.Draft(f.ctx, f.admin, f.org, "dina@x.io")
	require.NoError(t, err)
	f.clock = f.clock.Add(invite.DraftTTL)

	drafts, err := f.svc.ListDrafts(f.ctx, f.org)
	require.NoError(t, err)
	assert.Empty(t, drafts, "an expired draft is not listed")
	_, err = f.svc.ConfirmDraft(f.ctx, f.owner, f.org, d.ID)
	assert.True(t, invite.IsDraftExpiredError(err), "got %v", err)

	again, err := f.svc.Draft(f.ctx, f.admin, f.org, "dina@x.io")
	require.NoError(t, err)
	assert.Equal(t, d.ID, again.ID, "a new draft refreshes the old one")
	assert.Equal(t, f.clock.Add(invite.DraftTTL), again.ExpiresAt)
}

func TestService_DiscardDraft(t *testing.T) {
	f := newFixture(t)
	d, err := f.svc.Draft(f.ctx, f.admin, f.org, "dina@x.io")
	require.NoError(t, err)
	c := f.create("bob@x.io", member.RoleMember)

	assert.True(t, invite.IsNotFoundError(f.svc.DiscardDraft(f.ctx, f.admin, f.org, c.Invite.ID)), "discard only takes drafts")
	assert.True(t, invite.IsNotFoundError(f.svc.Revoke(f.ctx, f.admin, f.org, d.ID)), "revoke only takes invites")
	assert.True(t, authz.IsForbiddenError(f.svc.DiscardDraft(f.ctx, f.plain, f.org, d.ID)))
	require.NoError(t, f.svc.DiscardDraft(f.ctx, f.admin, f.org, d.ID))
	drafts, err := f.svc.ListDrafts(f.ctx, f.org)
	require.NoError(t, err)
	assert.Empty(t, drafts)
}

func TestService_ListPendingShowsExpired(t *testing.T) {
	f := newFixture(t)
	f.create("bob@x.io", member.RoleMember)
	f.clock = f.clock.Add(invite.TTL)
	pending, err := f.svc.ListPending(f.ctx, f.org)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, invite.StatusExpired, pending[0].Status(f.clock))
}

func TestService_ByPublicID(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	got, err := f.svc.ByPublicID(f.ctx, c.Invite.PublicID)
	require.NoError(t, err)
	assert.Equal(t, c.Invite.ID, got.ID)
	_, err = f.svc.ByPublicID(f.ctx, "nope")
	assert.True(t, invite.IsNotFoundError(err))
}

func TestService_FailuresAreReported(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	d, err := f.svc.Draft(f.ctx, f.owner, f.org, "dina@x.io")
	require.NoError(t, err)
	f.store.Err = errors.New("db down")

	_, err = f.svc.Create(f.ctx, f.owner, f.org, "eve@x.io", member.RoleMember)
	assert.True(t, isUnexpected(err), "create: %v", err)
	_, err = f.svc.Resend(f.ctx, f.owner, f.org, c.Invite.ID)
	assert.True(t, isUnexpected(err), "resend: %v", err)
	assert.True(t, isUnexpected(f.svc.Revoke(f.ctx, f.owner, f.org, c.Invite.ID)))
	_, err = f.svc.Draft(f.ctx, f.owner, f.org, "zed@x.io")
	assert.True(t, isUnexpected(err), "draft: %v", err)
	_, err = f.svc.ConfirmDraft(f.ctx, f.owner, f.org, d.ID)
	assert.True(t, isUnexpected(err), "confirm: %v", err)
	_, err = f.svc.Accept(f.ctx, c.Invite.ID, f.owner.UserID)
	assert.True(t, isUnexpected(err), "accept: %v", err)
	_, err = f.svc.ListPending(f.ctx, f.org)
	assert.True(t, isUnexpected(err), "list: %v", err)
	_, err = f.svc.ByPublicID(f.ctx, "x")
	assert.True(t, isUnexpected(err), "by public id: %v", err)
}

func TestService_MailFailureRollsBack(t *testing.T) {
	f := newFixture(t)
	f.mail.err = errors.New("queue down")
	_, err := f.svc.Create(f.ctx, f.owner, f.org, "bob@x.io", member.RoleMember)
	assert.True(t, isUnexpected(err), "got %v", err)
}

func TestService_AcceptUnknownPerson(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	_, err := f.svc.Accept(f.ctx, c.Invite.ID, uuid.New())
	assert.True(t, user.IsNotFoundError(err), "got %v", err)
}

func TestService_Join(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleAdmin)

	_, err := f.svc.Join(f.ctx, c.Invite.ID, "Bob", "short", true)
	assert.True(t, password.IsPolicyError(err), "a weak password: %v", err)
	_, err = f.users.ByEmail(f.ctx, "bob@x.io")
	assert.True(t, user.IsNotFoundError(err), "nothing is created")

	id, err := f.svc.Join(f.ctx, c.Invite.ID, " Bob ", "a sturdy passphrase", true)
	require.NoError(t, err)
	u, err := f.users.ByID(f.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "bob@x.io", u.Email, "the email comes from the invite")
	assert.Equal(t, "Bob", u.Name)
	require.NotNil(t, u.TermsAcceptedAt)
	m, err := f.members.Find(f.ctx, f.org, id)
	require.NoError(t, err)
	assert.Equal(t, member.RoleAdmin, m.Role)

	_, err = f.svc.Join(f.ctx, c.Invite.ID, "Bob", "a sturdy passphrase", true)
	assert.Equal(t, invite.ReasonUsed, invite.ReasonOf(err), "a used invite: %v", err)
}

func TestService_JoinWithoutTerms(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	id, err := f.svc.Join(f.ctx, c.Invite.ID, "Bob", "a sturdy passphrase", false)
	require.NoError(t, err)
	u, err := f.users.ByID(f.ctx, id)
	require.NoError(t, err)
	assert.Nil(t, u.TermsAcceptedAt)
}

func TestService_JoinNeverMakesASecondUser(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	_, err := f.users.Create(f.ctx, user.CreateRequest{Email: "bob@x.io", Password: "a sturdy passphrase"})
	require.NoError(t, err)
	_, err = f.svc.Join(f.ctx, c.Invite.ID, "Bob", "another sturdy passphrase", true)
	assert.True(t, user.IsAlreadyExistsError(err), "got %v", err)
	stored, err := f.store.ByID(f.ctx, c.Invite.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.AcceptedAt, "the invite stays usable for sign-in")
}

func TestService_JoinUnusable(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Join(f.ctx, uuid.New(), "Bob", "a sturdy passphrase", true)
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err))
	d, err := f.svc.Draft(f.ctx, f.owner, f.org, "dina@x.io")
	require.NoError(t, err)
	_, err = f.svc.Join(f.ctx, d.ID, "Dina", "a sturdy passphrase", true)
	assert.Equal(t, invite.ReasonNotFound, invite.ReasonOf(err), "a draft can't be joined")
	c := f.create("late@x.io", member.RoleMember)
	f.clock = f.clock.Add(invite.TTL)
	_, err = f.svc.Join(f.ctx, c.Invite.ID, "Late", "a sturdy passphrase", true)
	assert.Equal(t, invite.ReasonExpired, invite.ReasonOf(err))
}

type directory struct {
	people people
	users  *user.Service
}

func (d directory) PublicID(ctx context.Context, id uuid.UUID) (string, error) {
	if _, ok := d.people[id]; ok {
		return publicIDOf(id), nil
	}
	u, err := d.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	return u.PublicID, nil
}

func (d directory) Contact(ctx context.Context, id uuid.UUID) (string, string, error) {
	if c, ok := d.people[id]; ok {
		return c.email, c.name, nil
	}
	u, err := d.users.ByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	return u.Email, u.Name, nil
}

func TestService_EventsOnSend(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleAdmin)
	got := f.events.all()
	require.Len(t, got, 1)
	assert.Equal(t, f.org, got[0].org)
	assert.Equal(t, events.InviteCreated, got[0].t)
	assert.Equal(t, events.InviteCreatedV1{
		ID: c.Invite.PublicID, Email: "bob@x.io", Role: "admin", InvitedBy: publicIDOf(f.owner.UserID), ExpiresAt: c.Invite.ExpiresAt,
	}, got[0].data)

	_, err := f.svc.Resend(f.ctx, f.owner, f.org, c.Invite.ID)
	require.NoError(t, err)
	d, err := f.svc.Draft(f.ctx, f.owner, f.org, "dina@x.io")
	require.NoError(t, err)
	assert.Len(t, f.events.all(), 2, "a draft is not an invite yet")
	_, err = f.svc.ConfirmDraft(f.ctx, f.owner, f.org, d.ID)
	require.NoError(t, err)
	assert.Len(t, f.events.all(), 3, "a confirmed draft is")

	_, err = f.svc.Create(f.ctx, member.Requester{Operator: true}, f.org, "eve@x.io", member.RoleMember)
	require.NoError(t, err)
	last := f.events.all()[3].data.(events.InviteCreatedV1)
	assert.Empty(t, last.InvitedBy, "an operator is nobody's public id")
}

func TestService_EventsOnJoin(t *testing.T) {
	f := newFixture(t)
	c := f.create("bob@x.io", member.RoleMember)
	id, err := f.svc.Join(f.ctx, c.Invite.ID, "Bob", "a sturdy passphrase", true)
	require.NoError(t, err)
	u, err := f.users.ByID(f.ctx, id)
	require.NoError(t, err)
	got := f.events.all()
	require.Len(t, got, 2)
	assert.Equal(t, events.MemberAdded, got[1].t)
	assert.Equal(t, events.MemberAddedV1{UserID: u.PublicID, Email: "bob@x.io", Name: "Bob", Role: "member", InviteID: c.Invite.PublicID}, got[1].data)

	owned := f.create("olga@example.com", member.RoleMember)
	_, err = f.svc.Accept(f.ctx, owned.Invite.ID, f.owner.UserID)
	require.NoError(t, err)
	assert.Len(t, f.events.all(), 3, "an existing member was not added: no member.added")
}

func TestService_EventFailureFailsTheChange(t *testing.T) {
	f := newFixture(t)
	f.events.err = errors.New("outbox down")
	_, err := f.svc.Create(f.ctx, f.owner, f.org, "bob@x.io", member.RoleMember)
	assert.True(t, isUnexpected(err), "got %v", err)
}
