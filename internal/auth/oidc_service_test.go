package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
)

const testIssuer = "https://idp.example.com"

type fakeIdentities struct {
	users *fakeUsers
	race  func(email string)
}

func (f *fakeIdentities) ByIDP(_ context.Context, issuer, subject string) (*user.User, error) {
	for _, u := range f.users.byEmail {
		if u.IDPIssuer == issuer && u.IDPSubject == subject {
			return u, nil
		}
	}
	return nil, &user.NotFoundError{}
}

func (f *fakeIdentities) LinkIDP(_ context.Context, id uuid.UUID, issuer, subject string) (bool, error) {
	for _, u := range f.users.byEmail {
		if u.ID == id && u.IDPIssuer == "" {
			u.IDPIssuer, u.IDPSubject = issuer, subject
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeIdentities) CreateLinked(_ context.Context, email, name, issuer, subject string) (*user.User, error) {
	if f.race != nil {
		f.race(email)
		return nil, &user.AlreadyExistsError{Email: email}
	}
	u, err := user.New(email, name)
	if err != nil {
		return nil, err
	}
	u.IDPIssuer, u.IDPSubject = issuer, subject
	f.users.byEmail[u.Email] = u
	return u, nil
}

type fakeSeat struct {
	seat     Seat
	role     member.Role
	accepted bool
}

type fakeInvites struct {
	byPublicID map[string]*fakeSeat
	members    *fakeMembers
	users      *fakeUsers
	accepted   []uuid.UUID
}

func (f *fakeInvites) Seat(_ context.Context, publicID string) (*Seat, error) {
	if s, ok := f.byPublicID[publicID]; ok && !s.accepted {
		return &s.seat, nil
	}
	return nil, nil
}

func (f *fakeInvites) OwnerSeat(_ context.Context, orgID uuid.UUID, email string) (*Seat, error) {
	for _, s := range f.byPublicID {
		if s.seat.OrgID == orgID && strings.EqualFold(s.seat.Email, email) && !s.accepted {
			return &s.seat, nil
		}
	}
	return nil, nil
}

func (f *fakeInvites) Claim(_ context.Context, seat *Seat, userID uuid.UUID) (bool, error) {
	for _, s := range f.byPublicID {
		if s.seat.ID != seat.ID || s.accepted {
			continue
		}
		for _, u := range f.users.byEmail {
			if u.ID == userID && !strings.EqualFold(u.Email, s.seat.Email) {
				return false, nil
			}
		}
		s.accepted = true
		m, err := member.New(seat.OrgID, userID, s.role)
		if err != nil {
			return false, err
		}
		f.members.byUser[userID] = append(f.members.byUser[userID], m)
		f.accepted = append(f.accepted, seat.ID)
		return true, nil
	}
	return false, nil
}

type oidcFixture struct {
	*fixture
	invites *fakeInvites
	ids     *fakeIdentities
	sys     *org.Org
}

func newOIDCFixture(t *testing.T, openSignup bool) *oidcFixture {
	t.Helper()
	f := &oidcFixture{fixture: newFixture(t, BasicCredentials{})}
	f.sys = f.addOrg(t, true)
	f.addProject(f.sys)
	f.invites = &fakeInvites{byPublicID: map[string]*fakeSeat{}, members: f.members, users: f.users}
	d := f.svc.deps
	f.ids = &fakeIdentities{users: f.users}
	d.Identities, d.Invites, d.Issuer, d.OpenSignup = f.ids, f.invites, testIssuer, openSignup
	f.svc = NewService(d)
	return f
}

func (f *oidcFixture) invite(t *testing.T, o *org.Org, email string, role member.Role) (publicID string, id uuid.UUID) {
	t.Helper()
	publicID, id = "inv-"+uuid.NewString()[:8], uuid.New()
	f.invites.byPublicID[publicID] = &fakeSeat{seat: Seat{ID: id, OrgID: o.ID, Email: email}, role: role}
	return publicID, id
}

func oidcLogin(sub, email string) OIDCLogin {
	return OIDCLogin{Subject: sub, Email: email, EmailVerified: true, Name: "Ada", IDToken: "the.id.token"}
}

// SECURITY: OIDC Core §5.7, an account's link is keyed on the provider's sub; a verified email links an unlinked account once.
func TestLoginOIDC_LinksOnceThenBySubject(t *testing.T) {
	f := newOIDCFixture(t, false)
	ada := f.addUser(t, "ada@example.com", true)
	f.join(t, f.sys, ada, member.RoleOwner)

	p, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "Ada@Example.com"))
	require.NoError(t, err, "the provider's email is compared after normalization")
	assert.Equal(t, ada.ID, p.UserID)
	assert.Equal(t, session.SourceOIDC, p.Source)
	assert.Equal(t, testIssuer, p.IDPIssuer)
	assert.Equal(t, "sub-1", p.IDPSubject)
	assert.Equal(t, "the.id.token", p.IDToken, "kept for the RP-initiated logout's id_token_hint")
	assert.Equal(t, f.sys.ID, p.ActiveOrgID)

	again, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ada.new@example.com"))
	require.NoError(t, err, "the second time the sub alone matches, whatever the email now is")
	assert.Equal(t, ada.ID, again.UserID)

	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-2", "ada@example.com"))
	assert.True(t, IsIdentityConflictError(err), "a link is never re-pointed: %v", err)
	assert.Equal(t, "sub-1", ada.IDPSubject)
}

func TestLoginOIDC_Refusals(t *testing.T) {
	f := newOIDCFixture(t, false)
	ada := f.addUser(t, "ada@example.com", true)
	f.join(t, f.sys, ada, member.RoleOwner)

	unverified := oidcLogin("sub-1", "ada@example.com")
	unverified.EmailVerified = false
	_, err := f.svc.LoginOIDC(context.Background(), unverified)
	var none *NoConsoleAccountError
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleUnverified, none.Reason, "an unverified email links nothing, and says so")
	assert.Empty(t, ada.IDPSubject)

	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ada@example.com"))
	require.NoError(t, err)
	_, err = f.svc.LoginOIDC(context.Background(), unverified)
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleUnverified, none.Reason, "a linked sub with an unverified email gets the same clear reason")

	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("", "ada@example.com"))
	assert.True(t, IsNoConsoleAccountError(err))
	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-8", "not an email"))
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleNotInvited, none.Reason)

	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-9", "bob@example.com"))
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleNotInvited, none.Reason, "selfhosted never creates an account without an invite")
	assert.NotContains(t, f.users.byEmail, "bob@example.com")

	cara := f.addUser(t, "cara@example.com", false)
	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-3", "cara@example.com"))
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleRemoved, none.Reason, "an account in no org is removed from this deployment")
	assert.Equal(t, "sub-3", cara.IDPSubject)
}

// The email-only owner seat install leaves is claimed by the first verified sign-in with that email.
func TestLoginOIDC_ClaimsTheOwnerSeat(t *testing.T) {
	f := newOIDCFixture(t, false)
	_, seat := f.invite(t, f.sys, "owner@example.com", member.RoleOwner)

	_, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-9", "eve@example.com"))
	var none *NoConsoleAccountError
	require.ErrorAs(t, err, &none)
	assert.Equal(t, NoConsoleNotInvited, none.Reason, "another email can't claim the owner seat")

	p, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "Owner@Example.com"))
	require.NoError(t, err)
	owner := f.users.byEmail["owner@example.com"]
	require.NotNil(t, owner)
	assert.Equal(t, owner.ID, p.UserID)
	assert.Equal(t, "sub-1", owner.IDPSubject)
	assert.Empty(t, owner.PasswordHash, "an account made by an OIDC sign-in has no password")
	assert.Equal(t, []uuid.UUID{seat}, f.invites.accepted)
	assert.Equal(t, f.sys.ID, p.ActiveOrgID)
}

// An existing account in no org with the seat's email claims the seat too.
func TestLoginOIDC_ExistingAccountClaimsTheOwnerSeat(t *testing.T) {
	f := newOIDCFixture(t, false)
	ada := f.addUser(t, "ada@example.com", false)
	_, seat := f.invite(t, f.sys, "ada@example.com", member.RoleOwner)

	p, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ada@example.com"))
	require.NoError(t, err)
	assert.Equal(t, ada.ID, p.UserID)
	assert.Equal(t, []uuid.UUID{seat}, f.invites.accepted)
	assert.Equal(t, f.sys.ID, p.ActiveOrgID)

	again, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ada@example.com"))
	require.NoError(t, err, "the linked account signs in; the used seat is skipped")
	assert.Equal(t, ada.ID, again.UserID)
	assert.Len(t, f.invites.accepted, 1)
}

func TestLoginOIDC_CloudCreatesAnAccountWithNoOrg(t *testing.T) {
	f := newOIDCFixture(t, true)
	p, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ana@shop.test"))
	require.NoError(t, err)
	ana := f.users.byEmail["ana@shop.test"]
	require.NotNil(t, ana)
	assert.Equal(t, ana.ID, p.UserID)
	assert.Equal(t, testIssuer, ana.IDPIssuer)
	assert.Equal(t, uuid.Nil, p.ActiveOrgID, "onboarding creates the org")
	assert.True(t, p.IsPerson())
}

// A concurrent first sign-in that creates the account first is found again, never a 500.
func TestLoginOIDC_CreateRace(t *testing.T) {
	f := newOIDCFixture(t, true)
	f.ids.race = func(email string) {
		u, err := user.New(email, "Ana")
		require.NoError(t, err)
		u.IDPIssuer, u.IDPSubject = testIssuer, "sub-1"
		f.users.byEmail[u.Email] = u
	}
	p, err := f.svc.LoginOIDC(context.Background(), oidcLogin("sub-1", "ana@shop.test"))
	require.NoError(t, err, "the same sub won the race: that account signs in")
	assert.Equal(t, f.users.byEmail["ana@shop.test"].ID, p.UserID)

	f.ids.race = func(email string) {
		u, err := user.New(email, "Bo")
		require.NoError(t, err)
		u.IDPIssuer, u.IDPSubject = testIssuer, "sub-other"
		f.users.byEmail[u.Email] = u
	}
	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-2", "bo@shop.test"))
	assert.True(t, IsIdentityConflictError(err), "another sub took the email: AUT007, %v", err)

	f.ids.race = func(string) {}
	_, err = f.svc.LoginOIDC(context.Background(), oidcLogin("sub-3", "cy@shop.test"))
	assert.True(t, IsIdentityConflictError(err), "nothing to find again: AUT007, %v", err)
}

func TestLoginOIDC_FromAnInvite(t *testing.T) {
	f := newOIDCFixture(t, false)
	acme := f.addOrg(t, false)
	theirs, _ := f.invite(t, acme, "cara@example.com", member.RoleMember)

	in := oidcLogin("sub-1", "dan@example.com")
	in.Invite = theirs
	_, err := f.svc.LoginOIDC(context.Background(), in)
	assert.True(t, IsNoConsoleAccountError(err), "someone else's invite creates nothing")
	assert.Empty(t, f.invites.accepted)

	in = oidcLogin("sub-2", "Cara@Example.com")
	in.Invite = theirs
	p, err := f.svc.LoginOIDC(context.Background(), in)
	require.NoError(t, err, "the invite's email matches whatever its case")
	assert.Equal(t, acme.ID, p.ActiveOrgID)

	ada := f.addUser(t, "ada@example.com", true)
	f.join(t, f.sys, ada, member.RoleOwner)
	joins, joinsID := f.invite(t, acme, "ada@example.com", member.RoleAdmin)
	in = oidcLogin("sub-3", "ada@example.com")
	in.Invite = joins
	_, err = f.svc.LoginOIDC(context.Background(), in)
	require.NoError(t, err)
	assert.Contains(t, f.invites.accepted, joinsID, "an existing account joins the org it was invited to")
	assert.Len(t, f.members.byUser[ada.ID], 2)

	used := oidcLogin("sub-3", "ada@example.com")
	used.Invite = joins
	_, err = f.svc.LoginOIDC(context.Background(), used)
	require.NoError(t, err, "a used invite is skipped, the sign-in goes on")
	assert.Len(t, f.members.byUser[ada.ID], 2)
}

// Cloud signup is open, so someone else's invite opens an account without claiming it; the web then shows the invite's Mismatch.
func TestLoginOIDC_CloudInviteForAnotherEmail(t *testing.T) {
	f := newOIDCFixture(t, true)
	acme := f.addOrg(t, false)
	dans, _ := f.invite(t, acme, "dan@x.test", member.RoleMember)

	in := oidcLogin("sub-eve", "eve@x.test")
	in.Invite = dans
	p, err := f.svc.LoginOIDC(context.Background(), in)
	require.NoError(t, err)
	eve := f.users.byEmail["eve@x.test"]
	require.NotNil(t, eve)
	assert.Equal(t, eve.ID, p.UserID)
	assert.Empty(t, f.invites.accepted, "Dan's invite stays open")
	assert.Empty(t, f.members.byUser[eve.ID])

	other, err := f.svc.InviteMismatch(context.Background(), dans, "Eve@x.test")
	require.NoError(t, err)
	assert.True(t, other)
	other, err = f.svc.InviteMismatch(context.Background(), dans, "DAN@x.test")
	require.NoError(t, err)
	assert.False(t, other)
	other, err = f.svc.InviteMismatch(context.Background(), "inv-gone", "eve@x.test")
	require.NoError(t, err)
	assert.False(t, other, "a used, expired or unknown invite is no mismatch")

	self := newOIDCFixture(t, false)
	theirs, _ := self.invite(t, self.addOrg(t, false), "dan@x.test", member.RoleMember)
	in = oidcLogin("sub-eve", "eve@x.test")
	in.Invite = theirs
	_, err = self.svc.LoginOIDC(context.Background(), in)
	var none *NoConsoleAccountError
	require.ErrorAs(t, err, &none, "selfhosted creates no account")
	assert.Equal(t, NoConsoleNotInvited, none.Reason)
}
