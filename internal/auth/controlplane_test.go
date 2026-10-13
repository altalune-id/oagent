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
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

type orgByID map[uuid.UUID]*org.Org

func (m orgByID) Get(_ context.Context, id uuid.UUID) (*org.Org, error) {
	if o, ok := m[id]; ok {
		return o, nil
	}
	return nil, &org.NotFoundError{}
}

type projectByID map[uuid.UUID]*project.Project

func (m projectByID) Get(_ context.Context, id uuid.UUID) (*project.Project, error) {
	if p, ok := m[id]; ok {
		return p, nil
	}
	return nil, &project.NotFoundError{}
}

type userByID map[uuid.UUID]*user.User

func (m userByID) ByID(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u, ok := m[id]; ok {
		return u, nil
	}
	return nil, &user.NotFoundError{}
}

func whoamiClient(t *testing.T, orgs OrgGetter, projects ProjectGetter, users UserGetter, p session.Principal) authv1connect.AuthServiceClient {
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
	_, h := authv1connect.NewAuthServiceHandler(NewControlPlane(orgs, projects, users), connect.WithInterceptors(inject))
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
	acme := &org.Org{ID: uuid.New(), PublicID: "acmeOrgPublicId000001", Slug: "acme"}
	orgs := orgByID{acme.ID: acme}
	owner := &user.User{ID: uuid.New(), PublicID: "adaUserPublicId000001", Email: "ada@example.com", Name: "Ada"}
	users := userByID{owner.ID: owner}
	prj := &project.Project{ID: uuid.New(), PublicID: "webProjectPublicId001", OrgID: acme.ID}
	projects := projectByID{prj.ID: prj}

	t.Run("session", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			UserID: owner.ID, Email: owner.Email, Name: "Ada", AvatarURL: "https://x/a.png", Source: session.SourceLocal,
			ActiveOrgID: acme.ID, ActiveProjectID: prj.ID, Scopes: []string{"agent:*"}, Operator: true,
		}))
		require.NoError(t, err)
		assert.Equal(t, "local", got.GetSource())
		assert.Equal(t, owner.PublicID, got.GetUserId())
		assert.Equal(t, "ada@example.com", got.GetEmail())
		assert.Equal(t, "https://x/a.png", got.GetAvatarUrl())
		assert.Equal(t, acme.PublicID, got.GetActiveOrgId())
		assert.Equal(t, "acme", got.GetActiveOrgSlug())
		assert.Equal(t, prj.PublicID, got.GetActiveProjectId())
		assert.Equal(t, []string{"agent:*"}, got.GetPermissions())
		assert.True(t, got.GetOperator())
		assert.Empty(t, got.GetKeyHint())
	})

	t.Run("org key", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			KeyID: uuid.New(), KeyHint: "a1b2", Name: "ci", Source: session.SourceAPIKey, ActiveOrgID: acme.ID, Scopes: []string{"agent:read"},
		}))
		require.NoError(t, err)
		assert.Equal(t, "apikey", got.GetSource())
		assert.Equal(t, "a1b2", got.GetKeyHint())
		assert.Equal(t, "ci", got.GetName())
		assert.Empty(t, got.GetUserId())
		assert.Empty(t, got.GetOwnerId())
		assert.Equal(t, acme.PublicID, got.GetActiveOrgId())
		assert.Equal(t, "acme", got.GetActiveOrgSlug())
		assert.Equal(t, []string{"agent:read"}, got.GetPermissions())
		assert.False(t, got.GetOperator())
	})

	t.Run("personal key shows its owner", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			KeyID: uuid.New(), KeyHint: "c3d4", OwnerID: owner.ID, Name: "laptop", Source: session.SourceAPIKey, ActiveOrgID: acme.ID,
		}))
		require.NoError(t, err)
		assert.Equal(t, owner.PublicID, got.GetOwnerId())
		assert.Equal(t, "c3d4", got.GetKeyHint())
		assert.Equal(t, "ada@example.com", got.GetEmail())
		assert.Equal(t, "Ada", got.GetName())
		assert.Empty(t, got.GetUserId())
		assert.Empty(t, got.GetPermissions())
	})

	t.Run("basic", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			Source: session.SourceBasic, Name: "ops", ActiveOrgID: acme.ID, Scopes: []string{"*"}, Operator: true,
		}))
		require.NoError(t, err)
		assert.Equal(t, "basic", got.GetSource())
		assert.Equal(t, "ops", got.GetName())
		assert.True(t, got.GetOperator())
	})

	t.Run("none", func(t *testing.T) {
		_, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{}))
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("missing org leaves the id and slug empty", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{Source: session.SourceBasic, ActiveOrgID: uuid.New()}))
		require.NoError(t, err)
		assert.Empty(t, got.GetActiveOrgId())
		assert.Empty(t, got.GetActiveOrgSlug())
	})

	t.Run("missing project leaves its id empty", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			Source: session.SourceLocal, UserID: owner.ID, ActiveOrgID: acme.ID, ActiveProjectID: uuid.New(),
		}))
		require.NoError(t, err)
		assert.Empty(t, got.GetActiveProjectId())
		assert.Equal(t, acme.PublicID, got.GetActiveOrgId())
	})

	t.Run("missing user leaves its id empty", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			Source: session.SourceLocal, UserID: uuid.New(), Email: "gone@example.com",
		}))
		require.NoError(t, err)
		assert.Empty(t, got.GetUserId())
		assert.Equal(t, "gone@example.com", got.GetEmail())
	})

	t.Run("missing owner leaves its id empty", func(t *testing.T) {
		got, err := whoami(t, whoamiClient(t, orgs, projects, users, session.Principal{
			Source: session.SourceAPIKey, KeyHint: "e5f6", OwnerID: uuid.New(), Name: "laptop",
		}))
		require.NoError(t, err)
		assert.Empty(t, got.GetOwnerId())
		assert.Equal(t, "laptop", got.GetName())
	})
}

type brokenOrgs struct{}

func (brokenOrgs) Get(context.Context, uuid.UUID) (*org.Org, error) {
	return nil, errors.New("db down")
}

type brokenProjects struct{}

func (brokenProjects) Get(context.Context, uuid.UUID) (*project.Project, error) {
	return nil, errors.New("db down")
}

type brokenUsers struct{}

func (brokenUsers) ByID(context.Context, uuid.UUID) (*user.User, error) {
	return nil, errors.New("db down")
}

func TestWhoami_LookupErrorFails(t *testing.T) {
	for name, tc := range map[string]struct {
		orgs     OrgGetter
		projects ProjectGetter
		users    UserGetter
		p        session.Principal
	}{
		"org":     {brokenOrgs{}, projectByID{}, userByID{}, session.Principal{Source: session.SourceBasic, ActiveOrgID: uuid.New()}},
		"project": {orgByID{}, brokenProjects{}, userByID{}, session.Principal{Source: session.SourceBasic, ActiveProjectID: uuid.New()}},
		"user":    {orgByID{}, projectByID{}, brokenUsers{}, session.Principal{Source: session.SourceLocal, UserID: uuid.New()}},
		"owner":   {orgByID{}, projectByID{}, brokenUsers{}, session.Principal{Source: session.SourceAPIKey, OwnerID: uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := whoami(t, whoamiClient(t, tc.orgs, tc.projects, tc.users, tc.p))
			assert.Error(t, err)
		})
	}
}
