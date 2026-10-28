package auth

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
)

type switchFixture struct {
	*fixture
	person  *user.User
	home    *org.Org
	other   *org.Org
	foreign *org.Org
	sid     string
}

func newSwitchFixture(t *testing.T) *switchFixture {
	t.Helper()
	f := &switchFixture{fixture: newFixture(t, BasicCredentials{}), sid: "sid-1"}
	f.person = f.addUser(t, "ada@example.com", true)
	f.home, f.other, f.foreign = f.addOrg(t, false), f.addOrg(t, false), f.addOrg(t, false)
	f.join(t, f.home, f.person, member.RoleOwner)
	f.join(t, f.other, f.person, member.RoleMember)
	f.addProject(f.other)
	return f
}

func (f *switchFixture) signedIn(t *testing.T) session.Principal {
	t.Helper()
	p, err := f.svc.Refresh(context.Background(), principalOf(f.person, time.Now()))
	require.NoError(t, err)
	require.Equal(t, f.home.ID, p.ActiveOrgID, "a fresh session starts in the first membership")
	require.NoError(t, f.sessions.Save(context.Background(), f.sid, p, time.Now().Add(time.Hour)))
	return p
}

func (f *switchFixture) stored(t *testing.T) session.Principal {
	t.Helper()
	p, ok, err := f.sessions.Load(context.Background(), f.sid)
	require.NoError(t, err)
	require.True(t, ok)
	return p
}

func TestSetActiveOrg_AMemberSwitchesAndScopesFollowTheRole(t *testing.T) {
	f := newSwitchFixture(t)
	p := f.signedIn(t)

	got, err := f.svc.SetActiveOrg(context.Background(), f.sid, p, f.other.ID)
	require.NoError(t, err)
	assert.Equal(t, f.other.ID, got.ActiveOrgID)
	assert.Equal(t, f.projects.byOrg[f.other.ID][0].ID, got.ActiveProjectID, "the new org's first project becomes active")
	assert.Equal(t, member.BuiltinPermissions(member.RoleMember), got.Scopes, "scopes come from the role in the new org")

	assert.Equal(t, f.other.ID, f.stored(t).ActiveOrgID, "the session keeps the switch")
	again, err := f.svc.Refresh(context.Background(), f.stored(t))
	require.NoError(t, err)
	assert.Equal(t, f.other.ID, again.ActiveOrgID, "the next request stays in the new org")
}

// SECURITY: an org you do not belong to and an unknown org answer the same not-found, and the session never moves.
func TestSetActiveOrg_RefusesAnOrgYouDoNotBelongTo(t *testing.T) {
	f := newSwitchFixture(t)
	p := f.signedIn(t)

	for name, id := range map[string]uuid.UUID{"foreign": f.foreign.ID, "unknown": uuid.New(), "none": uuid.Nil} {
		_, err := f.svc.SetActiveOrg(context.Background(), f.sid, p, id)
		assert.True(t, org.IsNotFoundError(err), "%s: %v", name, err)
		assert.Equal(t, f.home.ID, f.stored(t).ActiveOrgID, name)
	}
}

func TestSetActiveOrg_OnlyAPersonWithALiveSessionSwitches(t *testing.T) {
	f := newSwitchFixture(t)
	p := f.signedIn(t)

	_, err := f.svc.SetActiveOrg(context.Background(), "gone", p, f.other.ID)
	assert.True(t, authn.IsUnauthorizedError(err), "a session that ended mid-request signs the person out: %v", err)

	key := session.Principal{Source: session.SourceAPIKey, ActiveOrgID: f.home.ID}
	_, err = f.svc.SetActiveOrg(context.Background(), f.sid, key, f.other.ID)
	assert.True(t, org.IsNotFoundError(err), "a key never switches: %v", err)
}

// An OIDC session switches orgs like a password one and keeps its ID token.
func TestSetActiveOrg_OIDCSession(t *testing.T) {
	f := newSwitchFixture(t)
	p := f.signedIn(t)
	p.Source, p.IDPIssuer, p.IDPSubject, p.IDToken = session.SourceOIDC, testIssuer, "sub-1", "the.id.token"

	got, err := f.svc.SetActiveOrg(context.Background(), f.sid, p, f.other.ID)
	require.NoError(t, err)
	assert.Equal(t, f.other.ID, got.ActiveOrgID)
	assert.Equal(t, session.SourceOIDC, got.Source)
	assert.Equal(t, "the.id.token", f.stored(t).IDToken)
}

func TestSetActiveOrgRPC_AnEndedSessionIsUnauthenticated(t *testing.T) {
	f := newSwitchFixture(t)
	p := f.signedIn(t)
	plane := NewControlPlane(orgByID{f.other.ID: f.other}, projectByID{}, userByID{f.person.ID: f.person}).WithProfiles(f.svc)
	require.NoError(t, f.sessions.Delete(context.Background(), f.sid))

	_, err := plane.SetActiveOrg(profileCtx(p, f.sid), connect.NewRequest(&authv1.SetActiveOrgRequest{OrgId: f.other.PublicID}))
	require.Error(t, err)
	assert.True(t, authn.IsUnauthorizedError(err), "%v", err)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnauthenticated, ae.Code(), "unauthenticated, not an internal error")
}
