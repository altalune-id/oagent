package auth

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
)

// Profiles changes the signed-in person's own account; *Service implements it.
type Profiles interface {
	UpdateProfile(ctx context.Context, userID uuid.UUID, name string) (*user.User, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, keepSID, current, next string) error
	SignOutEverywhere(ctx context.Context, userID uuid.UUID) error
}

// AuthService implements auth.v1.AuthService; every id it returns is a public id.
type AuthService struct {
	orgs     OrgGetter
	projects ProjectGetter
	users    UserGetter
	profiles Profiles
}

var _ authv1connect.AuthServiceHandler = (*AuthService)(nil)

// NewControlPlane binds the handler to the org, project and user lookups whoami fills names and public ids from.
func NewControlPlane(orgs OrgGetter, projects ProjectGetter, users UserGetter) *AuthService {
	return &AuthService{orgs: orgs, projects: projects, users: users}
}

// WithProfiles serves the profile RPCs over p.
func (s *AuthService) WithProfiles(p Profiles) *AuthService {
	s.profiles = p
	return s
}

// UpdateProfile sets the signed-in person's name.
func (s *AuthService) UpdateProfile(
	ctx context.Context,
	req *connect.Request[authv1.UpdateProfileRequest],
) (*connect.Response[authv1.UpdateProfileResponse], error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.profiles.UpdateProfile(ctx, p.UserID, req.Msg.GetName())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.UpdateProfileResponse{UserId: u.PublicID, Name: u.Name}), nil
}

// ChangePassword sets a new password after checking the current one and signs out every other session.
func (s *AuthService) ChangePassword(
	ctx context.Context,
	req *connect.Request[authv1.ChangePasswordRequest],
) (*connect.Response[authv1.ChangePasswordResponse], error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	sid, _ := session.SIDFrom(ctx)
	if err := s.profiles.ChangePassword(ctx, p.UserID, sid, req.Msg.GetCurrentPassword(), req.Msg.GetNewPassword()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.ChangePasswordResponse{}), nil
}

// SignOutEverywhere ends every session of the signed-in person, this one too.
func (s *AuthService) SignOutEverywhere(
	ctx context.Context,
	_ *connect.Request[authv1.SignOutEverywhereRequest],
) (*connect.Response[authv1.SignOutEverywhereResponse], error) {
	p, err := person(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.profiles.SignOutEverywhere(ctx, p.UserID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.SignOutEverywhereResponse{SignedOut: true}), nil
}

// SECURITY: keys, basic auth and the system principal never change a person's account.
func person(ctx context.Context) (session.Principal, error) {
	p := session.PrincipalFrom(ctx)
	switch {
	case p.Source == "":
		return session.Principal{}, &authn.UnauthorizedError{}
	case !p.IsPerson():
		return session.Principal{}, &authz.ForbiddenError{}
	}
	return p, nil
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
			out.UserId, out.Name = u.PublicID, u.Name
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
