package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tokens"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/scope"
)

type fakeVerifier struct {
	p   session.Principal
	err error
}

func (f fakeVerifier) Verify(context.Context, string) (session.Principal, error) { return f.p, f.err }

type failingIdentities struct{ Identities }

func (failingIdentities) ByIDP(context.Context, string, string) (*user.User, error) {
	return nil, errors.New("db down")
}

type failingMembers struct{}

func (failingMembers) ListByUser(context.Context, uuid.UUID) ([]*member.Membership, error) {
	return nil, errors.New("db down")
}

func tokenFor(subject string, granted ...string) session.Principal {
	return session.Principal{Source: session.SourceToken, IDPIssuer: testIssuer, IDPSubject: subject, Scopes: granted}
}

func (f *oidcFixture) linked(t *testing.T, email, subject string) *user.User {
	t.Helper()
	u := f.addUser(t, email, false)
	u.IDPIssuer, u.IDPSubject = testIssuer, subject
	return u
}

func (f *oidcFixture) token(t *testing.T, granted session.Principal) (session.Principal, error) {
	t.Helper()
	a := TokenAuthenticator{Verifier: fakeVerifier{p: granted}, Auth: f.svc}
	return a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindToken, Token: "a.b.c"})
}

func TestTokenAuthenticator_RefusesOtherKinds(t *testing.T) {
	f := newOIDCFixture(t, false)
	a := TokenAuthenticator{Verifier: fakeVerifier{p: tokenFor("sub-1")}, Auth: f.svc}
	for _, k := range []authn.Kind{authn.KindNone, authn.KindAPIKey, authn.KindBasic, authn.KindSession} {
		_, err := a.Authenticate(context.Background(), authn.Credential{Kind: k, Token: "a.b.c"})
		assert.Error(t, err, k)
	}
}

func TestTokenAuthenticator_PassesTheVerifierRefusalOn(t *testing.T) {
	f := newOIDCFixture(t, false)
	for _, want := range []error{&tokens.InvalidTokenError{}, &authn.UnavailableError{}} {
		a := TokenAuthenticator{Verifier: fakeVerifier{err: want}, Auth: f.svc}
		_, err := a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindToken, Token: "a.b.c"})
		assert.ErrorIs(t, err, want)
	}
}

func TestToken_UnknownSubjectIsUnauthorized(t *testing.T) {
	f := newOIDCFixture(t, false)
	f.addUser(t, "ada@example.com", true)

	_, err := f.token(t, tokenFor("sub-never-signed-in", scope.AgentRead))
	assert.True(t, authn.IsUnauthorizedError(err), "err = %v", err)
}

func TestToken_StoreFailureIsUnavailable(t *testing.T) {
	f := newOIDCFixture(t, false)
	d := f.svc.deps
	d.Identities = failingIdentities{d.Identities}
	f.svc = NewService(d)
	_, err := f.token(t, tokenFor("sub-1"))
	assert.True(t, authn.IsUnavailableError(err), "err = %v", err)

	f = newOIDCFixture(t, false)
	f.linked(t, "ada@example.com", "sub-1")
	d = f.svc.deps
	d.Members = failingMembers{}
	f.svc = NewService(d)
	_, err = f.token(t, tokenFor("sub-1"))
	assert.True(t, authn.IsUnavailableError(err), "err = %v", err)
}

func TestToken_OneOrgSetsTheTenant(t *testing.T) {
	f := newOIDCFixture(t, false)
	u := f.linked(t, "ada@example.com", "sub-1")
	o := f.addOrg(t, false)
	f.join(t, o, u, member.RoleAdmin)
	first := f.addProject(o)
	f.addProject(o)

	p, err := f.token(t, tokenFor("sub-1", scope.AgentRead))
	require.NoError(t, err)
	assert.Equal(t, session.SourceToken, p.Source)
	assert.Equal(t, u.ID, p.UserID)
	assert.Equal(t, u.Email, p.Email)
	assert.Equal(t, o.ID, p.ActiveOrgID)
	assert.Equal(t, first.ID, p.ActiveProjectID)
	assert.True(t, p.IsPerson())
}

func TestToken_SeveralOrgsLeaveItUntenanted(t *testing.T) {
	f := newOIDCFixture(t, false)
	u := f.linked(t, "ada@example.com", "sub-1")
	for range 2 {
		o := f.addOrg(t, false)
		f.join(t, o, u, member.RoleOwner)
		f.addProject(o)
	}

	p, err := f.token(t, tokenFor("sub-1", scope.AgentRead, scope.MemberRead))
	require.NoError(t, err)
	assert.Equal(t, u.ID, p.UserID)
	assert.Equal(t, uuid.Nil, p.ActiveOrgID, "never a guessed org")
	assert.Equal(t, uuid.Nil, p.ActiveProjectID)
	assert.Empty(t, p.Scopes)
}

func TestToken_NoOrgIsUntenanted(t *testing.T) {
	f := newOIDCFixture(t, false)
	f.linked(t, "ada@example.com", "sub-1")

	p, err := f.token(t, tokenFor("sub-1", scope.OrgRead))
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, p.ActiveOrgID)
	assert.Empty(t, p.Scopes)
}

func TestToken_ScopesAreGrantedAndRole(t *testing.T) {
	f := newOIDCFixture(t, false)
	u := f.linked(t, "ada@example.com", "sub-1")
	o := f.addOrg(t, false)
	f.join(t, o, u, member.RoleMember)
	f.addProject(o)

	granted := []string{"openid", "*", "agent:*", scope.AgentRead, scope.OrgWrite, scope.MemberWrite, scope.ApikeyWrite, scope.MemberRead}
	p, err := f.token(t, tokenFor("sub-1", granted...))
	require.NoError(t, err)
	assert.Equal(t, []string{scope.AgentRead, scope.MemberRead}, p.Scopes, "granted ∩ the member role ∩ what a token may hold")
	assert.Equal(t, granted, p.TokenScopes, "the granted list is kept for the step-up challenge")
}

func TestToken_NeverAnOperator(t *testing.T) {
	f := newOIDCFixture(t, false)
	u := f.linked(t, "ada@example.com", "sub-1")
	f.join(t, f.sys, u, member.RoleOwner)

	p, err := f.token(t, tokenFor("sub-1", scope.MemberWrite))
	require.NoError(t, err)
	assert.Equal(t, f.sys.ID, p.ActiveOrgID)
	assert.False(t, p.Operator, "a system-org owner's token stays inside the scope check")
}

func TestToken_RefreshKeepsTheOnlyOrgRule(t *testing.T) {
	f := newOIDCFixture(t, false)
	u := f.linked(t, "ada@example.com", "sub-1")
	for range 2 {
		f.join(t, f.addOrg(t, false), u, member.RoleOwner)
	}
	stale := session.Principal{Source: session.SourceToken, UserID: u.ID, TokenScopes: []string{scope.AgentRead}}

	p, err := f.svc.Refresh(context.Background(), stale)
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, p.ActiveOrgID, "a token never falls into the first-org rule of a web session")
}
