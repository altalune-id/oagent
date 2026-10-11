package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
)

func TestSessionAuthenticator_RebuildsAuthority(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	u := f.addUser(t, "ada@example.com", true)
	o := f.addOrg(t, false)
	m := f.join(t, o, u, member.RoleAdmin)
	f.addProject(o)

	stored := session.Principal{UserID: u.ID, Email: u.Email, Source: session.SourceLocal, ActiveOrgID: o.ID}
	require.NoError(t, f.sessions.Save(context.Background(), "sid-1", stored, time.Now().Add(time.Hour)))
	m.Role = member.RoleMember

	a := SessionAuthenticator{Sessions: f.sessions, Auth: f.svc}
	p, err := a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindSession, Token: "sid-1"})
	require.NoError(t, err)
	assert.Equal(t, u.ID, p.UserID)
	assert.Equal(t, member.BuiltinPermissions(member.RoleMember), p.Scopes)
}

func TestSessionAuthenticator_Refuses(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	a := SessionAuthenticator{Sessions: f.sessions, Auth: f.svc}

	for name, c := range map[string]authn.Credential{
		"unknown sid": {Kind: authn.KindSession, Token: "missing"},
		"api key":     {Kind: authn.KindAPIKey, Token: "oagent_x"},
		"basic":       {Kind: authn.KindBasic, User: "a", Password: "b"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := a.Authenticate(context.Background(), c)
			assert.True(t, IsInvalidCredentialsError(err), "got %v", err)
		})
	}
}

type brokenStore struct{ session.Store }

func (brokenStore) Load(context.Context, string) (session.Principal, bool, error) {
	return session.Principal{}, false, errors.New("db down")
}

func TestSessionAuthenticator_StoreError(t *testing.T) {
	f := newFixture(t, BasicCredentials{})
	a := SessionAuthenticator{Sessions: brokenStore{}, Auth: f.svc}
	_, err := a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindSession, Token: "sid"})
	require.ErrorContains(t, err, "db down")
}

func TestBasicAuthenticator(t *testing.T) {
	f := newFixture(t, BasicCredentials{User: "admin", Password: "s3cret-pass"})
	sys := f.addOrg(t, true)
	a := BasicAuthenticator{Auth: f.svc}

	p, err := a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindBasic, User: "admin", Password: "s3cret-pass"})
	require.NoError(t, err)
	assert.Equal(t, session.SourceBasic, p.Source)
	assert.True(t, p.Operator)
	assert.Equal(t, sys.ID, p.ActiveOrgID)
	assert.Equal(t, uuid.Nil, p.UserID)

	_, err = a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindBasic, User: "admin", Password: "wrong"})
	assert.True(t, IsInvalidCredentialsError(err))

	_, err = a.Authenticate(context.Background(), authn.Credential{Kind: authn.KindSession, Token: "sid"})
	assert.True(t, IsInvalidCredentialsError(err))
}
