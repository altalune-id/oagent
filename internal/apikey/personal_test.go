package apikey_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

type roleKey struct{ org, user uuid.UUID }

type fakeOwners struct {
	roles map[roleKey]member.Role
	err   error
}

func (f *fakeOwners) Role(_ context.Context, orgID, userID uuid.UUID) (member.Role, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	r, ok := f.roles[roleKey{orgID, userID}]
	return r, ok, nil
}

func (f *fakeOwners) set(orgID, userID uuid.UUID, r member.Role) {
	if f.roles == nil {
		f.roles = map[roleKey]member.Role{}
	}
	f.roles[roleKey{orgID, userID}] = r
}

func TestScheme_MintPersonal(t *testing.T) {
	orgID, owner, prj := uuid.New(), uuid.New(), uuid.New()
	k, plain, err := apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", []string{scope.AgentRead},
		apikey.ProjectGrant{ProjectIDs: []uuid.UUID{prj}}, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.Equal(t, apikey.KindPersonal, k.Kind)
	assert.Equal(t, owner, k.CreatedBy)
	assert.True(t, k.HasGrant())
	assert.Equal(t, []uuid.UUID{prj}, k.ReachableProjects())

	_, _, err = apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", []string{"apikey:write"},
		apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	assert.True(t, apikey.IsPersonOnlyScopeError(err))
}

func personalService(store apikey.Store, owners apikey.OwnerRoles, enabled bool) *apikey.Service {
	return newService(store, stubProjects{}).WithOwnerRoles(owners).WithPersonalKeys(enabled)
}

func personalReq(orgID, owner uuid.UUID, scopes ...string) apikey.CreateRequest {
	return apikey.CreateRequest{
		OrgID: orgID, Kind: apikey.KindPersonal, Name: "laptop", Scopes: scopes,
		AllProjects: true, ExpiresAt: future(), CreatedBy: owner,
	}
}

func TestService_CreatePersonal_WithinRole(t *testing.T) {
	orgID, owner := uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, owner, member.RoleAdmin)
	store := &fakes.APIKeyStore{}

	k, _, err := personalService(store, owners, true).Create(context.Background(), personalReq(orgID, owner, "project:write", "agent:write"))
	require.NoError(t, err)
	assert.Equal(t, apikey.KindPersonal, k.Kind)
	assert.Equal(t, owner, k.CreatedBy)
	assert.Same(t, k, store.Saved)
}

func TestService_CreatePersonal_AboveRole(t *testing.T) {
	orgID, owner := uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, owner, member.RoleMember)

	_, _, err := personalService(&fakes.APIKeyStore{}, owners, true).Create(context.Background(), personalReq(orgID, owner, "agent:write", "project:write"))
	require.True(t, apikey.IsScopeExceedsRoleError(err), "got %v", err)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeAPIKeyScopeExceedsRole, ae.Code())
	assert.Equal(t, apperror.KindForbidden, apperror.KindOf(ae.Code()))
}

func TestService_CreatePersonal_NotAMember(t *testing.T) {
	_, _, err := personalService(&fakes.APIKeyStore{}, &fakeOwners{}, true).Create(context.Background(), personalReq(uuid.New(), uuid.New(), "agent:read"))
	assert.True(t, apikey.IsScopeExceedsRoleError(err))
}

func TestService_CreatePersonal_NeedsOwner(t *testing.T) {
	_, _, err := personalService(&fakes.APIKeyStore{}, &fakeOwners{}, true).Create(context.Background(), personalReq(uuid.New(), uuid.Nil, "agent:read"))
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeValidation, ae.Code())
}

func TestService_CreatePersonal_Disabled(t *testing.T) {
	orgID, owner := uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, owner, member.RoleOwner)

	for name, svc := range map[string]*apikey.Service{
		"capability off": personalService(&fakes.APIKeyStore{}, owners, false),
		"no owner port":  newService(&fakes.APIKeyStore{}, stubProjects{}).WithPersonalKeys(true),
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.Create(context.Background(), personalReq(orgID, owner, "agent:read"))
			require.True(t, apikey.IsPersonalKeysDisabledError(err), "got %v", err)
			ae, _ := apperror.AsAppError(err)
			assert.Equal(t, apperror.KindFailedPrecondition, apperror.KindOf(ae.Code()))
		})
	}
}

func TestService_CreatePersonal_RoleLookupError(t *testing.T) {
	_, _, err := personalService(&fakes.APIKeyStore{}, &fakeOwners{err: errors.New("db down")}, true).
		Create(context.Background(), personalReq(uuid.New(), uuid.New(), "agent:read"))
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
}

func personalKey(t *testing.T, orgID, owner uuid.UUID, scopes ...string) (*apikey.APIKey, string) {
	t.Helper()
	k, plain, err := apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", scopes, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	return k, plain
}

func TestService_List_HidesOthersPersonalKeys(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	mine, _ := personalKey(t, orgID, me)
	theirs, _ := personalKey(t, orgID, other)
	machine, _, err := apikey.NewScheme("").MintOrg(orgID, "ci", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	svc := newService(&fakes.APIKeyStore{Keys: []*apikey.APIKey{mine, theirs, machine}}, stubProjects{})

	got, err := svc.List(context.Background(), orgID, me)
	require.NoError(t, err)
	assert.ElementsMatch(t, []*apikey.APIKey{mine, machine}, got)

	all, err := svc.List(context.Background(), orgID, uuid.Nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestService_Revoke_OthersPersonalKeyIsNotFound(t *testing.T) {
	orgID, me, other := uuid.New(), uuid.New(), uuid.New()
	theirs, _ := personalKey(t, orgID, other)
	store := &fakes.APIKeyStore{Key: theirs}

	err := newService(store, stubProjects{}).Revoke(context.Background(), orgID, theirs.ID, me, true)
	assert.True(t, apikey.IsNotFoundError(err))
	assert.Nil(t, store.Saved)

	require.NoError(t, newService(store, stubProjects{}).Revoke(context.Background(), orgID, theirs.ID, other, false))
	assert.NotNil(t, store.Saved.RevokedAt)
}

func TestAuthenticator_PersonalKeyFollowsOwnerRole(t *testing.T) {
	orgID, owner := uuid.New(), uuid.New()
	k, plain := personalKey(t, orgID, owner, "project:write", "agent:write")
	owners := &fakeOwners{}
	owners.set(orgID, owner, member.RoleAdmin)
	a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), owners, nil)

	p, err := a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"project:write", "agent:write"}, p.Scopes)
	assert.Equal(t, owner, p.OwnerID)
	assert.Equal(t, uuid.Nil, p.UserID)
	assert.False(t, p.Operator)

	owners.set(orgID, owner, member.RoleMember)
	p, err = a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)
	assert.Equal(t, []string{"agent:write"}, p.Scopes)

	delete(owners.roles, roleKey{orgID, owner})
	_, err = a.Authenticate(context.Background(), keyCred(plain))
	assert.True(t, authn.IsUnauthorizedError(err))
}

func TestAuthenticator_PersonalKeyRefusals(t *testing.T) {
	orgID, owner := uuid.New(), uuid.New()

	t.Run("owner deleted", func(t *testing.T) {
		k, plain := personalKey(t, orgID, owner, "agent:read")
		k.CreatedBy = uuid.Nil
		owners := &fakeOwners{}
		owners.set(orgID, uuid.Nil, member.RoleOwner)
		a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), owners, nil)
		_, err := a.Authenticate(context.Background(), keyCred(plain))
		assert.True(t, authn.IsUnauthorizedError(err))
	})
	t.Run("no owner port", func(t *testing.T) {
		k, plain := personalKey(t, orgID, owner, "agent:read")
		a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), nil, nil)
		_, err := a.Authenticate(context.Background(), keyCred(plain))
		assert.True(t, authn.IsUnauthorizedError(err))
	})
	t.Run("role lookup fails", func(t *testing.T) {
		k, plain := personalKey(t, orgID, owner, "agent:read")
		r := &reported{}
		a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), &fakeOwners{err: errors.New("db down")}, r.fn)
		_, err := a.Authenticate(context.Background(), keyCred(plain))
		assert.True(t, authn.IsUnauthorizedError(err))
		assert.Len(t, r.errs, 1)
	})
}

func TestService_MachineKeyNeverExceedsCreatorRole(t *testing.T) {
	orgID, admin, prj := uuid.New(), uuid.New(), uuid.New()
	owners := &fakeOwners{}
	owners.set(orgID, admin, member.RoleAdmin)
	svc := newService(&fakes.APIKeyStore{}, stubProjects{ids: []uuid.UUID{prj}}).WithOwnerRoles(owners).WithPersonalKeys(true)

	_, _, err := svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: orgID, Kind: apikey.KindOrg, Name: "ci", Scopes: []string{"org:write"}, AllProjects: true, ExpiresAt: future(), CreatedBy: admin,
	})
	assert.True(t, apikey.IsScopeExceedsRoleError(err), "org key above the admin role: %v", err)

	_, _, err = svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: orgID, Kind: apikey.KindOrg, Name: "ci", Scopes: []string{"agent:write"}, AllProjects: true, ExpiresAt: future(), CreatedBy: admin,
	})
	require.NoError(t, err, "within the role is fine")

	_, _, err = svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: orgID, Kind: apikey.KindOrg, Name: "ci", Scopes: []string{"org:write"}, AllProjects: true, ExpiresAt: future(),
	})
	require.NoError(t, err, "a trusted caller (no creator) is not capped")
}

func TestService_MachineKeyCreatorMustBeMember(t *testing.T) {
	svc := newService(&fakes.APIKeyStore{}, stubProjects{}).WithOwnerRoles(&fakeOwners{})
	_, _, err := svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "ci", Scopes: []string{"agent:read"}, AllProjects: true, ExpiresAt: future(), CreatedBy: uuid.New(),
	})
	assert.True(t, apikey.IsScopeExceedsRoleError(err))
}

func TestService_RevokePersonalOf(t *testing.T) {
	store := &fakes.APIKeyStore{RevokeCount: 2}
	orgID, ownerID := uuid.New(), uuid.New()
	n, err := newService(store, stubProjects{}).RevokePersonalOf(context.Background(), orgID, ownerID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	require.Len(t, store.Revocations, 1)
	got := store.Revocations[0]
	assert.Equal(t, orgID, got.OrgID)
	assert.Equal(t, ownerID, got.OwnerID)
	assert.WithinDuration(t, time.Now(), got.At, time.Minute)

	store.Err = errors.New("db down")
	_, err = newService(store, stubProjects{}).RevokePersonalOf(context.Background(), orgID, ownerID)
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok, "got %v", err)
}
