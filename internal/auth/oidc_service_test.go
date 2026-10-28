package auth

import (
	"context"
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

type oidcFixture struct {
	*fixture
	ids *fakeIdentities
	sys *org.Org
}

func newOIDCFixture(t *testing.T, openSignup bool) *oidcFixture {
	t.Helper()
	f := &oidcFixture{fixture: newFixture(t, BasicCredentials{})}
	f.sys = f.addOrg(t, true)
	f.addProject(f.sys)
	d := f.svc.deps
	f.ids = &fakeIdentities{users: f.users}
	d.Identities, d.Issuer, d.OpenSignup = f.ids, testIssuer, openSignup
	f.svc = NewService(d)
	return f
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
