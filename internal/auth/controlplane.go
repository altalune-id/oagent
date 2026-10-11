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
	"altalune.id/oagent/internal/user"
)

// AuthService implements auth.v1.AuthService.
type AuthService struct {
	orgs  OrgGetter
	users UserGetter
}

var _ authv1connect.AuthServiceHandler = (*AuthService)(nil)

// NewControlPlane binds the handler to the org and user lookups whoami fills names from.
func NewControlPlane(orgs OrgGetter, users UserGetter) *AuthService {
	return &AuthService{orgs: orgs, users: users}
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
		Source:          string(p.Source),
		UserId:          idString(p.UserID),
		Email:           p.Email,
		Name:            p.Name,
		AvatarUrl:       p.AvatarURL,
		KeyId:           idString(p.KeyID),
		OwnerId:         idString(p.OwnerID),
		ActiveOrgId:     idString(p.ActiveOrgID),
		ActiveProjectId: idString(p.ActiveProjectID),
		Permissions:     p.Scopes,
		Operator:        p.Operator,
	}
	if p.ActiveOrgID != uuid.Nil {
		o, err := s.orgs.Get(ctx, p.ActiveOrgID)
		switch {
		case err == nil:
			out.ActiveOrgSlug = o.Slug
		case !org.IsNotFoundError(err):
			return nil, err
		}
	}
	if p.OwnerID != uuid.Nil {
		u, err := s.users.ByID(ctx, p.OwnerID)
		switch {
		case err == nil:
			out.Email, out.Name, out.AvatarUrl = u.Email, u.Name, u.AvatarURL
		case !user.IsNotFoundError(err):
			return nil, err
		}
	}
	return connect.NewResponse(out), nil
}

func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
