package apikey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
)

func withPrincipal(p session.Principal) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if p.Source != "" {
				ctx = session.PrincipalInto(ctx, p)
			}
			return next(ctx, req)
		}
	}
}

func newPersonalClient(t *testing.T, refs *fakes.PublicIDs, store apikey.Store, owners apikey.OwnerRoles, p session.Principal) apikeyv1connect.APIKeyServiceClient {
	t.Helper()
	svc := apikey.NewService(store, apikey.NewScheme(""), stubProjects{}, testUnexpected()).WithOwnerRoles(owners).WithPersonalKeys(true)
	_, handler := apikeyv1connect.NewAPIKeyServiceHandler(apikey.NewControlPlane(svc, refs),
		connect.WithInterceptors(controlplane.Wrap(nil), withPrincipal(p)))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return apikeyv1connect.NewAPIKeyServiceClient(http.DefaultClient, ts.URL)
}

func person(userID, orgID uuid.UUID) session.Principal {
	return session.Principal{UserID: userID, Source: session.SourceLocal, ActiveOrgID: orgID}
}

func withRole(p session.Principal, role member.Role) session.Principal {
	p.Scopes = member.BuiltinPermissions(role)
	return p
}

func personalCreate(orgPub string) *connect.Request[apikeyv1.CreateAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgPub, Kind: "personal", Name: "laptop", Scopes: []string{"agent:read"},
		AllProjects: true, ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	})
}

func TestAPIKeyService_CreatePersonal_NeedsPerson(t *testing.T) {
	orgID := uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	for name, p := range map[string]session.Principal{
		"api key": {Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID},
		"basic":   {Source: session.SourceBasic, Operator: true, ActiveOrgID: orgID},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakes.APIKeyStore{}
			_, err := newPersonalClient(t, refs, store, &fakeOwners{}, p).CreateAPIKey(context.Background(), personalCreate(orgPub))
			require.Error(t, err)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			assert.Nil(t, store.Saved)
		})
	}
}

func TestAPIKeyService_CreatePersonal_ByPerson(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	store := &fakes.APIKeyStore{}

	resp, err := newPersonalClient(t, refs, store, owners, person(me, orgID)).CreateAPIKey(context.Background(), personalCreate(orgPub))
	require.NoError(t, err)
	assert.Equal(t, "personal", resp.Msg.GetKey().GetKind())
	assert.Equal(t, me, store.Saved.CreatedBy)
}

func TestAPIKeyService_CreatePersonal_AboveRoleIsForbidden(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	req := personalCreate(orgPub)
	req.Msg.Scopes = []string{"project:write"}

	_, err := newPersonalClient(t, refs, &fakes.APIKeyStore{}, owners, person(me, orgID)).CreateAPIKey(context.Background(), req)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestAPIKeyService_Create_RecordsCreator(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	store := &fakes.APIKeyStore{}
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleAdmin)
	_, err := newPersonalClient(t, refs, store, owners, withRole(person(me, orgID), member.RoleAdmin)).CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgPub, Kind: "org", Name: "ci", AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.Equal(t, me, store.Saved.CreatedBy)
}

func TestAPIKeyService_List_Visibility(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	mine, _ := personalKey(t, orgID, me)
	theirs, _ := personalKey(t, orgID, other)
	machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	keys := []*apikey.APIKey{mine, theirs, machine}

	cases := map[string]struct {
		p    session.Principal
		want []string
	}{
		"person sees own personal keys": {person(me, orgID), []string{mine.SecretHint, machine.SecretHint}},
		"key sees no personal keys":     {session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgID, AllProjects: true}, []string{machine.SecretHint}},
		"operator sees all":             {session.Principal{Source: session.SourceBasic, Operator: true}, []string{mine.SecretHint, theirs.SecretHint, machine.SecretHint}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := newPersonalClient(t, refs, &fakes.APIKeyStore{Keys: keys}, &fakeOwners{}, tc.p)
			resp, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgPub}))
			require.NoError(t, err)
			var got []string
			for _, k := range resp.Msg.GetKeys() {
				got = append(got, k.GetSecretHint())
			}
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

func TestAPIKeyService_Revoke_OthersPersonalKey(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	theirs, _ := personalKey(t, orgID, other)
	store := &fakes.APIKeyStore{Key: theirs}

	_, err := newPersonalClient(t, refs, store, &fakeOwners{}, person(me, orgID)).RevokeAPIKey(context.Background(),
		connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgPub, Hint: theirs.SecretHint}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func TestAPIKeyService_ReachIsTheActiveOrg(t *testing.T) {
	orgA, orgB, me := uuid.New(), uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	refs.Org(orgA)
	pubB := refs.Org(orgB)
	machine, _, err := apikey.NewScheme("").MintOrg(orgB, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: machine, Keys: []*apikey.APIKey{machine}}
	owners := &fakeOwners{}
	owners.set(orgB, me, member.RoleOwner)
	c := newPersonalClient(t, refs, store, owners, person(me, orgA))

	for _, org := range []string{pubB, orgB.String()} {
		_, err = c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: org}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), org)
	}
	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: pubB, Hint: machine.SecretHint}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.CreateAPIKey(context.Background(), personalCreate(pubB))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func orgCreate(orgPub string) *connect.Request[apikeyv1.CreateAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgPub, Kind: "org", Name: "ci", Scopes: []string{"agent:read"}, AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	})
}

func revoke(orgPub, hint string) *connect.Request[apikeyv1.RevokeAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgPub, Hint: hint})
}

// Review Focus 4: a member manages their own personal keys and nothing wider.
func TestAPIKeyService_MemberManagesOnlyOwnPersonalKeys(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	owners.set(orgID, other, member.RoleMember)
	p := withRole(person(me, orgID), member.RoleMember)

	t.Run("creates a personal key within the role", func(t *testing.T) {
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, refs, store, owners, p).CreateAPIKey(context.Background(), personalCreate(orgPub))
		require.NoError(t, err)
		assert.Equal(t, apikey.KindPersonal, store.Saved.Kind)
	})
	t.Run("cannot put a scope above the role on it", func(t *testing.T) {
		for _, sc := range []string{"org:write", "project:write"} {
			req := personalCreate(orgPub)
			req.Msg.Scopes = []string{sc}
			store := &fakes.APIKeyStore{}
			_, err := newPersonalClient(t, refs, store, owners, p).CreateAPIKey(context.Background(), req)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), sc)
			assert.Nil(t, store.Saved, sc)
		}
	})
	t.Run("cannot put apikey:write on it", func(t *testing.T) {
		req := personalCreate(orgPub)
		req.Msg.Scopes = []string{"apikey:write"}
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, refs, store, owners, p).CreateAPIKey(context.Background(), req)
		require.Error(t, err)
		assert.Nil(t, store.Saved)
	})
	t.Run("cannot mint an org key", func(t *testing.T) {
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, refs, store, owners, p).CreateAPIKey(context.Background(), orgCreate(orgPub))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
	t.Run("revokes their own personal key", func(t *testing.T) {
		mine, _ := personalKey(t, orgID, me)
		store := &fakes.APIKeyStore{Key: mine}
		_, err := newPersonalClient(t, refs, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgPub, mine.SecretHint))
		require.NoError(t, err)
		assert.NotNil(t, store.Saved.RevokedAt)
	})
	t.Run("cannot revoke an org key", func(t *testing.T) {
		machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
		require.NoError(t, err)
		store := &fakes.APIKeyStore{Key: machine}
		_, err = newPersonalClient(t, refs, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgPub, machine.SecretHint))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
	t.Run("cannot revoke another member's personal key", func(t *testing.T) {
		theirs, _ := personalKey(t, orgID, other)
		store := &fakes.APIKeyStore{Key: theirs}
		_, err := newPersonalClient(t, refs, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgPub, theirs.SecretHint))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
}

func TestAPIKeyService_AdminCannotRevokeAMembersPersonalKey(t *testing.T) {
	orgID, admin, other := uuid.New(), uuid.New(), uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	theirs, _ := personalKey(t, orgID, other)
	store := &fakes.APIKeyStore{Key: theirs}
	_, err := newPersonalClient(t, refs, store, &fakeOwners{}, withRole(person(admin, orgID), member.RoleAdmin)).
		RevokeAPIKey(context.Background(), revoke(orgPub, theirs.SecretHint))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

// SECURITY: a key never manages keys; the handler refuses it before the service runs.
func TestAPIKeyService_KeyPrincipalNeverReachesTheService(t *testing.T) {
	orgID := uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	for name, p := range map[string]session.Principal{
		"org key":      {Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID, AllProjects: true, Scopes: []string{"agent:read"}},
		"personal key": {Source: session.SourceAPIKey, KeyID: uuid.New(), OwnerID: uuid.New(), ActiveOrgID: orgID, AllProjects: true, Scopes: []string{"agent:read"}},
	} {
		t.Run(name, func(t *testing.T) {
			// NOTE: the store fails every read, so reaching the service would answer NotFound, not these codes.
			store := &fakes.APIKeyStore{Err: &apikey.NotFoundError{ID: "x"}}
			c := newPersonalClient(t, refs, store, &fakeOwners{}, p)

			_, err := c.CreateAPIKey(context.Background(), personalCreate(orgPub))
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			_, err = c.CreateAPIKey(context.Background(), orgCreate(orgPub))
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			_, err = c.RevokeAPIKey(context.Background(), revoke(orgPub, "abcd"))
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			assert.Nil(t, store.Saved)
		})
	}
}

func TestAPIKeyService_NoPrincipalReachesNothing(t *testing.T) {
	orgID := uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: machine, Keys: []*apikey.APIKey{machine}}
	c := newPersonalClient(t, refs, store, &fakeOwners{}, session.Principal{})

	_, err = c.CreateAPIKey(context.Background(), orgCreate(orgPub))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.CreateAPIKey(context.Background(), personalCreate(orgPub))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgPub}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.RevokeAPIKey(context.Background(), revoke(orgPub, machine.SecretHint))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func TestAPIKeyService_OperatorManagesKeys(t *testing.T) {
	orgID := uuid.New()
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	op := session.Principal{Source: session.SourceBasic, Operator: true, ActiveOrgID: orgID}
	store := &fakes.APIKeyStore{}
	_, err := newPersonalClient(t, refs, store, &fakeOwners{}, op).CreateAPIKey(context.Background(), orgCreate(orgPub))
	require.NoError(t, err)
}
