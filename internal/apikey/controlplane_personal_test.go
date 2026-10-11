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

func personalCreate(orgID uuid.UUID) *connect.Request[apikeyv1.CreateAPIKeyRequest] {
	return connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgID.String(), Kind: "personal", Name: "laptop", Scopes: []string{"agent:read"},
		AllProjects: true, ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	})
}

func TestAPIKeyService_CreatePersonal_NeedsPerson(t *testing.T) {
	orgID := uuid.New()
	for name, p := range map[string]session.Principal{
		"no principal": {},
		"api key":      {Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID},
		"basic":        {Source: session.SourceBasic, Operator: true, ActiveOrgID: orgID},
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
	_, err := newPersonalClient(t, store, owners, person(me, orgID)).CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
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
		"no principal sees all":         {session.Principal{}, []string{mine.ID.String(), theirs.ID.String(), machine.ID.String()}},
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
