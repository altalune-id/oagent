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

func newPersonalClient(t *testing.T, store apikey.Store, owners apikey.OwnerRoles, p session.Principal) apikeyv1connect.APIKeyServiceClient {
	t.Helper()
	svc := apikey.NewService(store, apikey.NewScheme(""), stubProjects{}, testUnexpected()).WithOwnerRoles(owners).WithPersonalKeys(true)
	_, handler := apikeyv1connect.NewAPIKeyServiceHandler(apikey.NewControlPlane(svc),
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

func personalCreate(orgID uuid.UUID) *connect.Request[apikeyv1.CreateAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgID.String(), Kind: "personal", Name: "laptop", Scopes: []string{"agent:read"},
		AllProjects: true, ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	})
}

func TestAPIKeyService_CreatePersonal_NeedsPerson(t *testing.T) {
	orgID := uuid.New()
	for name, p := range map[string]session.Principal{
		"api key": {Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID},
		"basic":   {Source: session.SourceBasic, Operator: true, ActiveOrgID: orgID},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakes.APIKeyStore{}
			_, err := newPersonalClient(t, store, &fakeOwners{}, p).CreateAPIKey(context.Background(), personalCreate(orgID))
			require.Error(t, err)
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			assert.Nil(t, store.Saved)
		})
	}
}

func TestAPIKeyService_CreatePersonal_ByPerson(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	store := &fakes.APIKeyStore{}

	resp, err := newPersonalClient(t, store, owners, person(me, orgID)).CreateAPIKey(context.Background(), personalCreate(orgID))
	require.NoError(t, err)
	assert.Equal(t, "personal", resp.Msg.GetKey().GetKind())
	assert.Equal(t, me, store.Saved.CreatedBy)
}

func TestAPIKeyService_CreatePersonal_AboveRoleIsForbidden(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	req := personalCreate(orgID)
	req.Msg.Scopes = []string{"project:write"}

	_, err := newPersonalClient(t, &fakes.APIKeyStore{}, owners, person(me, orgID)).CreateAPIKey(context.Background(), req)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestAPIKeyService_Create_RecordsCreator(t *testing.T) {
	orgID, me := uuid.New(), uuid.New()
	store := &fakes.APIKeyStore{}
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleAdmin)
	_, err := newPersonalClient(t, store, owners, withRole(person(me, orgID), member.RoleAdmin)).CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgID.String(), Kind: "org", Name: "ci", AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.Equal(t, me, store.Saved.CreatedBy)
}

func TestAPIKeyService_List_Visibility(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	mine, _ := personalKey(t, orgID, me)
	theirs, _ := personalKey(t, orgID, other)
	machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	keys := []*apikey.APIKey{mine, theirs, machine}

	cases := map[string]struct {
		p    session.Principal
		want []string
	}{
		"person sees own personal keys": {person(me, orgID), []string{mine.ID.String(), machine.ID.String()}},
		"key sees no personal keys":     {session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgID}, []string{machine.ID.String()}},
		"operator sees all":             {session.Principal{Source: session.SourceBasic, Operator: true}, []string{mine.ID.String(), theirs.ID.String(), machine.ID.String()}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := newPersonalClient(t, &fakes.APIKeyStore{Keys: keys}, &fakeOwners{}, tc.p)
			resp, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgID.String()}))
			require.NoError(t, err)
			var got []string
			for _, k := range resp.Msg.GetKeys() {
				got = append(got, k.GetId())
			}
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

func TestAPIKeyService_Revoke_OthersPersonalKey(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	theirs, _ := personalKey(t, orgID, other)
	store := &fakes.APIKeyStore{Key: theirs}

	_, err := newPersonalClient(t, store, &fakeOwners{}, person(me, orgID)).RevokeAPIKey(context.Background(),
		connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgID.String(), Id: theirs.ID.String()}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func TestAPIKeyService_ReachIsTheActiveOrg(t *testing.T) {
	orgA, orgB, me := uuid.New(), uuid.New(), uuid.New()
	machine, _, err := apikey.NewScheme("").MintOrg(orgB, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: machine, Keys: []*apikey.APIKey{machine}}
	owners := &fakeOwners{}
	owners.set(orgB, me, member.RoleOwner)
	c := newPersonalClient(t, store, owners, person(me, orgA))

	_, err = c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgB.String()}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgB.String(), Id: machine.ID.String()}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.CreateAPIKey(context.Background(), personalCreate(orgB))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func orgCreate(orgID uuid.UUID) *connect.Request[apikeyv1.CreateAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgID.String(), Kind: "org", Name: "ci", Scopes: []string{"agent:read"}, AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	})
}

func revoke(orgID, id uuid.UUID) *connect.Request[apikeyv1.RevokeAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgID.String(), Id: id.String()})
}

// Review Focus 4: a member manages their own personal keys and nothing wider.
func TestAPIKeyService_MemberManagesOnlyOwnPersonalKeys(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, me, member.RoleMember)
	owners.set(orgID, other, member.RoleMember)
	p := withRole(person(me, orgID), member.RoleMember)

	t.Run("creates a personal key within the role", func(t *testing.T) {
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, store, owners, p).CreateAPIKey(context.Background(), personalCreate(orgID))
		require.NoError(t, err)
		assert.Equal(t, apikey.KindPersonal, store.Saved.Kind)
	})
	t.Run("cannot put a scope above the role on it", func(t *testing.T) {
		for _, sc := range []string{"org:write", "project:write"} {
			req := personalCreate(orgID)
			req.Msg.Scopes = []string{sc}
			store := &fakes.APIKeyStore{}
			_, err := newPersonalClient(t, store, owners, p).CreateAPIKey(context.Background(), req)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), sc)
			assert.Nil(t, store.Saved, sc)
		}
	})
	t.Run("cannot put apikey:write on it", func(t *testing.T) {
		req := personalCreate(orgID)
		req.Msg.Scopes = []string{"apikey:write"}
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, store, owners, p).CreateAPIKey(context.Background(), req)
		require.Error(t, err)
		assert.Nil(t, store.Saved)
	})
	t.Run("cannot mint an org key", func(t *testing.T) {
		store := &fakes.APIKeyStore{}
		_, err := newPersonalClient(t, store, owners, p).CreateAPIKey(context.Background(), orgCreate(orgID))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
	t.Run("revokes their own personal key", func(t *testing.T) {
		mine, _ := personalKey(t, orgID, me)
		store := &fakes.APIKeyStore{Key: mine}
		_, err := newPersonalClient(t, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgID, mine.ID))
		require.NoError(t, err)
		assert.NotNil(t, store.Saved.RevokedAt)
	})
	t.Run("cannot revoke an org key", func(t *testing.T) {
		machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
		require.NoError(t, err)
		store := &fakes.APIKeyStore{Key: machine}
		_, err = newPersonalClient(t, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgID, machine.ID))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
	t.Run("cannot revoke another member's personal key", func(t *testing.T) {
		theirs, _ := personalKey(t, orgID, other)
		store := &fakes.APIKeyStore{Key: theirs}
		_, err := newPersonalClient(t, store, owners, p).RevokeAPIKey(context.Background(), revoke(orgID, theirs.ID))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.Nil(t, store.Saved)
	})
}

func TestAPIKeyService_AdminCannotRevokeAMembersPersonalKey(t *testing.T) {
	orgID, admin, other := uuid.New(), uuid.New(), uuid.New()
	theirs, _ := personalKey(t, orgID, other)
	store := &fakes.APIKeyStore{Key: theirs}
	_, err := newPersonalClient(t, store, &fakeOwners{}, withRole(person(admin, orgID), member.RoleAdmin)).
		RevokeAPIKey(context.Background(), revoke(orgID, theirs.ID))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

// SECURITY: a key never manages keys; the handler refuses it before the service runs.
func TestAPIKeyService_KeyPrincipalNeverReachesTheService(t *testing.T) {
	orgID := uuid.New()
	for name, p := range map[string]session.Principal{
		"org key":      {Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID, AllProjects: true, Scopes: []string{"agent:read"}},
		"personal key": {Source: session.SourceAPIKey, KeyID: uuid.New(), OwnerID: uuid.New(), ActiveOrgID: orgID, AllProjects: true, Scopes: []string{"agent:read"}},
	} {
		t.Run(name, func(t *testing.T) {
			// NOTE: the store fails every read, so reaching the service would answer NotFound, not these codes.
			store := &fakes.APIKeyStore{Err: &apikey.NotFoundError{ID: "x"}}
			c := newPersonalClient(t, store, &fakeOwners{}, p)

			_, err := c.CreateAPIKey(context.Background(), personalCreate(orgID))
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			_, err = c.CreateAPIKey(context.Background(), orgCreate(orgID))
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			_, err = c.RevokeAPIKey(context.Background(), revoke(orgID, uuid.New()))
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			assert.Nil(t, store.Saved)
		})
	}
}

func TestAPIKeyService_NoPrincipalReachesNothing(t *testing.T) {
	orgID := uuid.New()
	machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: machine, Keys: []*apikey.APIKey{machine}}
	c := newPersonalClient(t, store, &fakeOwners{}, session.Principal{})

	_, err = c.CreateAPIKey(context.Background(), orgCreate(orgID))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.CreateAPIKey(context.Background(), personalCreate(orgID))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgID.String()}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.RevokeAPIKey(context.Background(), revoke(orgID, machine.ID))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func TestAPIKeyService_OperatorManagesKeys(t *testing.T) {
	orgID := uuid.New()
	op := session.Principal{Source: session.SourceBasic, Operator: true, ActiveOrgID: orgID}
	store := &fakes.APIKeyStore{}
	_, err := newPersonalClient(t, store, &fakeOwners{}, op).CreateAPIKey(context.Background(), orgCreate(orgID))
	require.NoError(t, err)
}
