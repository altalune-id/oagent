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
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
)

type reported struct{ errs []error }

func (r *reported) fn(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
	r.errs = append(r.errs, cause)
	return apperror.New(apperror.CodeUnexpected, msg)
}

func mintOrgKey(t *testing.T, scopes []string) (*apikey.APIKey, string) {
	t.Helper()
	exp := time.Now().UTC().Add(time.Hour)
	k, plain, err := apikey.NewScheme("").MintOrg(uuid.New(), "ci", scopes, apikey.ProjectGrant{All: true}, &exp, time.Now().UTC())
	require.NoError(t, err)
	return k, plain
}

func keyCred(plain string) authn.Credential {
	return authn.Credential{Kind: authn.KindAPIKey, Token: plain}
}

func TestAuthenticator_OrgKey(t *testing.T) {
	k, plain := mintOrgKey(t, []string{"agent:read", "agent:write"})
	store := &fakes.APIKeyStore{Key: k}
	a := apikey.NewAuthenticator(store, nil, apikey.NewScheme(""), nil, nil)

	p, err := a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)
	assert.Equal(t, session.SourceAPIKey, p.Source)
	assert.Equal(t, k.ID, p.KeyID)
	assert.Equal(t, "ci", p.Name)
	assert.Equal(t, uuid.Nil, p.UserID)
	assert.Equal(t, k.OrgID, p.ActiveOrgID)
	assert.True(t, p.AllProjects)
	assert.Equal(t, []string{"agent:read", "agent:write"}, p.Scopes)
	assert.False(t, p.Operator)
	assert.WithinDuration(t, time.Now(), p.IssuedAt, time.Minute)
}

func TestAuthenticator_ProjectKey(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour)
	orgID, projectID, res := uuid.New(), uuid.New(), uuid.New()
	k, plain, err := apikey.NewScheme("").MintProject(orgID, projectID, "bot", []string{"agent:read"}, []uuid.UUID{res}, &exp, time.Now().UTC())
	require.NoError(t, err)
	a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), nil, nil)

	p, err := a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)
	assert.Equal(t, projectID, p.ActiveProjectID)
	assert.Equal(t, []uuid.UUID{projectID}, p.ProjectIDs)
	assert.Equal(t, []uuid.UUID{res}, p.ResourceIDs)
	assert.False(t, p.AllProjects)
}

func TestAuthenticator_Refuses(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	cases := map[string]func(k *apikey.APIKey){
		"revoked": func(k *apikey.APIKey) { k.Revoke(past) },
		"expired": func(k *apikey.APIKey) { k.ExpiresAt = &past },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			k, plain := mintOrgKey(t, []string{"agent:read"})
			mutate(k)
			a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), nil, nil)
			_, err := a.Authenticate(context.Background(), keyCred(plain))
			assert.True(t, authn.IsUnauthorizedError(err), "got %v", err)
		})
	}
	t.Run("unknown key", func(t *testing.T) {
		k, _ := mintOrgKey(t, []string{"agent:read"})
		_, other := mintOrgKey(t, []string{"agent:read"})
		a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), nil, nil)
		_, err := a.Authenticate(context.Background(), keyCred(other))
		assert.True(t, authn.IsUnauthorizedError(err))
	})
}

func TestAuthenticator_WrongShapeSkipsStore(t *testing.T) {
	store := &fakes.APIKeyStore{}
	a := apikey.NewAuthenticator(store, nil, apikey.NewScheme(""), nil, nil)

	for name, c := range map[string]authn.Credential{
		"wrong prefix": {Kind: authn.KindAPIKey, Token: "other_abc"},
		"session":      {Kind: authn.KindSession, Token: "oagent_abc"},
		"basic":        {Kind: authn.KindBasic, User: "u", Password: "p"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := a.Authenticate(context.Background(), c)
			assert.True(t, authn.IsUnauthorizedError(err))
		})
	}
	assert.Equal(t, 0, store.HashLookup)
}

func TestAuthenticator_StoreErrorIsReported(t *testing.T) {
	r := &reported{}
	_, plain := mintOrgKey(t, []string{"agent:read"})
	a := apikey.NewAuthenticator(&fakes.APIKeyStore{Err: errors.New("db down")}, nil, apikey.NewScheme(""), nil, r.fn)

	_, err := a.Authenticate(context.Background(), keyCred(plain))
	assert.True(t, authn.IsUnauthorizedError(err))
	require.Len(t, r.errs, 1)
	assert.ErrorContains(t, r.errs[0], "db down")
}

func TestAuthenticator_NotFoundIsNotReported(t *testing.T) {
	r := &reported{}
	_, plain := mintOrgKey(t, []string{"agent:read"})
	a := apikey.NewAuthenticator(&fakes.APIKeyStore{}, nil, apikey.NewScheme(""), nil, r.fn)

	_, err := a.Authenticate(context.Background(), keyCred(plain))
	assert.True(t, authn.IsUnauthorizedError(err))
	assert.Empty(t, r.errs)
}

func TestAuthenticator_StripsPersonOnlyScopes(t *testing.T) {
	k, plain := mintOrgKey(t, []string{"agent:read"})
	k.Scopes = append(k.Scopes, "apikey:write", "member:write")
	a := apikey.NewAuthenticator(&fakes.APIKeyStore{Key: k}, nil, apikey.NewScheme(""), nil, nil)

	p, err := a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)
	assert.Equal(t, []string{"agent:read"}, p.Scopes)
}

func TestAuthenticator_RecordsUsage(t *testing.T) {
	k, plain := mintOrgKey(t, []string{"agent:read"})
	store := &fakes.APIKeyStore{Key: k}
	usage := apikey.NewUsageWorker(store, time.Hour, nil)
	a := apikey.NewAuthenticator(store, usage, apikey.NewScheme(""), nil, nil)

	_, err := a.Authenticate(context.Background(), keyCred(plain))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, usage.Run(ctx), context.Canceled)

	batches := store.Batches()
	require.Len(t, batches, 1)
	assert.Contains(t, batches[0], k.ID)
}
