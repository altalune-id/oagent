package auth

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

// AuthService implements auth.v1.AuthService; every id it returns is a public id.
type AuthService struct {
	orgs     OrgGetter
	projects ProjectGetter
	users    UserGetter
}

var _ authv1connect.AuthServiceHandler = (*AuthService)(nil)

// NewControlPlane binds the handler to the org, project and user lookups whoami fills names and public ids from.
func NewControlPlane(orgs OrgGetter, projects ProjectGetter, users UserGetter) *AuthService {
	return &AuthService{orgs: orgs, projects: projects, users: users}
}

// Whoami describes the request's Principal; a request without one is unauthenticated.
func (s *AuthService) Whoami(
	ctx context.Context,
	_ *connect.Request[authv1.WhoamiRequest],
) (*connect.Response[authv1.WhoamiResponse], error) {
	p := session.PrincipalFrom(ctx)
	if p.Source == "" {
		return nil, &authn.UnauthorizedError{}
	}
	out := &authv1.WhoamiResponse{
		Source:      string(p.Source),
		Email:       p.Email,
		Name:        p.Name,
		AvatarUrl:   p.AvatarURL,
		KeyHint:     p.KeyHint,
		Permissions: p.Scopes,
		Operator:    p.Operator,
	}
	if p.UserID != uuid.Nil {
		u, err := s.users.ByID(ctx, p.UserID)
		switch {
		case err == nil:
			out.UserId = u.PublicID
		case !user.IsNotFoundError(err):
			return nil, err
		}
	}
	if p.ActiveOrgID != uuid.Nil {
		o, err := s.orgs.Get(ctx, p.ActiveOrgID)
		switch {
		case err == nil:
			out.ActiveOrgId, out.ActiveOrgSlug = o.PublicID, o.Slug
		case !org.IsNotFoundError(err):
			return nil, err
		}
	}
	if p.ActiveProjectID != uuid.Nil {
		pr, err := s.projects.Get(ctx, p.ActiveProjectID)
		switch {
		case err == nil:
			out.ActiveProjectId = pr.PublicID
		case !project.IsNotFoundError(err):
			return nil, err
		}
	}
	if p.OwnerID != uuid.Nil {
		u, err := s.users.ByID(ctx, p.OwnerID)
		switch {
		case err == nil:
			out.OwnerId = u.PublicID
			out.Email, out.Name, out.AvatarUrl = u.Email, u.Name, u.AvatarURL
		case !user.IsNotFoundError(err):
			return nil, err
		}
	}
	return connect.NewResponse(out), nil
}
