package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/user"
)

type orgByID map[uuid.UUID]*org.Org

func (m orgByID) Get(_ context.Context, id uuid.UUID) (*org.Org, error) {
	if o, ok := m[id]; ok {
		return o, nil
	}
	return nil, &org.NotFoundError{}
}

type userByID map[uuid.UUID]*user.User

func (m userByID) ByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u, ok := m[id]; ok {
		return u, nil
	}
	return nil, &user.NotFoundError{}
}

func whoamiClient(t *testing.T, orgs OrgGetter, users UserGetter, p session.Principal) authv1connect.AuthServiceClient {
	t.Helper()
	inject := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if p.Source != "" {
				ctx = session.PrincipalInto(ctx, p)
			}
			resp, err := next(ctx, req)
			if authn.IsUnauthorizedError(err) {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}
			return resp, err
		}
	})
	_, h := authv1connect.NewAuthServiceHandler(NewControlPlane(orgs, users), connect.WithInterceptors(inject))
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return authv1connect.NewAuthServiceClient(http.DefaultClient, ts.URL)
}

func whoami(t *testing.T, c authv1connect.AuthServiceClient) (*authv1.WhoamiResponse, error) {
	t.Helper()
	resp, err := c.Whoami(context.Background(), connect.NewRequest(&authv1.WhoamiRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestWhoami(t *testing.T) {
	acme := &org.Org{ID: uuid.New(), Slug: "acme"}
	orgs := orgByID{acme.ID: acme}
	owner := &user.User{ID: uuid.New(), Email: "ada@example.com", Name: "Ada"}
	users := userByID{owner.ID: owner}
	prj := uuid.New()

	t.Run("session", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{
			UserID: owner.ID, Email: owner.Email, Name: "Ada", AvatarURL: "https://x/a.png", Source: session.SourceLocal,
			ActiveOrgID: acme.ID, ActiveProjectID: prj, Scopes: []string{"agent:*"}, Operator: true,
		}))
		require.NoError(t, err)
		assert.Equal(t, "local", got.GetSource())
		assert.Equal(t, owner.ID.String(), got.GetUserId())
		assert.Equal(t, "ada@example.com", got.GetEmail())
		assert.Equal(t, "https://x/a.png", got.GetAvatarUrl())
		assert.Equal(t, "acme", got.GetActiveOrgSlug())
		assert.Equal(t, prj.String(), got.GetActiveProjectId())
		assert.Equal(t, []string{"agent:*"}, got.GetPermissions())
		assert.True(t, got.GetOperator())
		assert.Empty(t, got.GetKeyId())
	})

	t.Run("org key", func(t *testing.T) {
		key := uuid.New()
		got, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{
			KeyID: key, Name: "ci", Source: session.SourceAPIKey, ActiveOrgID: acme.ID, Scopes: []string{"agent:read"},
		}))
		require.NoError(t, err)
		assert.Equal(t, "apikey", got.GetSource())
		assert.Equal(t, key.String(), got.GetKeyId())
		assert.Equal(t, "ci", got.GetName())
		assert.Empty(t, got.GetUserId())
		assert.Empty(t, got.GetOwnerId())
		assert.Equal(t, "acme", got.GetActiveOrgSlug())
		assert.Equal(t, []string{"agent:read"}, got.GetPermissions())
		assert.False(t, got.GetOperator())
	})

	t.Run("personal key shows its owner", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{
			KeyID: uuid.New(), OwnerID: owner.ID, Name: "laptop", Source: session.SourceAPIKey, ActiveOrgID: acme.ID,
		}))
		require.NoError(t, err)
		assert.Equal(t, owner.ID.String(), got.GetOwnerId())
		assert.Equal(t, "ada@example.com", got.GetEmail())
		assert.Equal(t, "Ada", got.GetName())
		assert.Empty(t, got.GetUserId())
		assert.Empty(t, got.GetPermissions())
	})

	t.Run("basic", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{
			Source: session.SourceBasic, Name: "ops", ActiveOrgID: acme.ID, Scopes: []string{"*"}, Operator: true,
		}))
		require.NoError(t, err)
		assert.Equal(t, "basic", got.GetSource())
		assert.Equal(t, "ops", got.GetName())
		assert.True(t, got.GetOperator())
	})

	t.Run("none", func(t *testing.T) {
		_, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{}))
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("missing org leaves the slug empty", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, users, session.Principal{Source: session.SourceBasic, ActiveOrgID: uuid.New()}))
		require.NoError(t, err)
		assert.Empty(t, got.GetActiveOrgSlug())
	})
}

type brokenOrgs struct{}

func (brokenOrgs) Get(context.Context, uuid.UUID) (*org.Org, error) {
	return nil, errors.New("db down")
}

func TestWhoami_LookupErrorFails(t *testing.T) {
	_, err := whoami(t, whoamiClient(t, brokenOrgs{}, userByID{}, session.Principal{Source: session.SourceBasic, ActiveOrgID: uuid.New()}))
	assert.Error(t, err)
}
