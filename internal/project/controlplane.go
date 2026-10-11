package project

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/kernel/authz"
)

// ProjectService implements project.v1.ProjectService.
type ProjectService struct {
	projects *Service
}

// NewControlPlane binds the handler to the project service.
func NewControlPlane(projects *Service) *ProjectService {
	return &ProjectService{projects: projects}
}

var _ projectv1connect.ProjectServiceHandler = (*ProjectService)(nil)

// CreateProject creates a project within an organization, minting a slug when one is not supplied.
func (s *ProjectService) CreateProject(
	ctx context.Context,
	req *connect.Request[projectv1.CreateProjectRequest],
) (*connect.Response[projectv1.CreateProjectResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	if !authz.ReachWholeOrg(ctx, orgID) {
		return nil, &NotFoundError{}
	}
	p, err := s.projects.Create(ctx, CreateRequest{OrgID: orgID, Slug: req.Msg.GetSlug(), Name: req.Msg.GetName()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.CreateProjectResponse{Project: projectToProto(p)}), nil
}

// GetProject loads a project by id.
func (s *ProjectService) GetProject(
	ctx context.Context,
	req *connect.Request[projectv1.GetProjectRequest],
) (*connect.Response[projectv1.GetProjectResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	p, err := s.reachable(ctx, id, authz.ReachProject)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.GetProjectResponse{Project: projectToProto(p)}), nil
}

// ListProjects lists the organization's projects the caller reaches, ordered by creation time.
func (s *ProjectService) ListProjects(
	ctx context.Context,
	req *connect.Request[projectv1.ListProjectsRequest],
) (*connect.Response[projectv1.ListProjectsResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	if !authz.ReachOrg(ctx, orgID) {
		return nil, &NotFoundError{}
	}
	projects, err := s.projects.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*projectv1.Project, 0, len(projects))
	for _, p := range projects {
		if authz.ReachProject(ctx, p.OrgID, p.ID) {
			out = append(out, projectToProto(p))
		}
	}
	return connect.NewResponse(&projectv1.ListProjectsResponse{Projects: out}), nil
}

// UpdateProject renames a project and/or changes its slug.
func (s *ProjectService) UpdateProject(
	ctx context.Context,
	req *connect.Request[projectv1.UpdateProjectRequest],
) (*connect.Response[projectv1.UpdateProjectResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	if _, err := s.reachable(ctx, id, authz.ReachWholeProject); err != nil {
		return nil, err
	}
	p, err := s.projects.Update(ctx, id, req.Msg.GetName(), req.Msg.GetSlug())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.UpdateProjectResponse{Project: projectToProto(p)}), nil
}

// SECURITY: a project outside the caller's reach is reported as absent.
func (s *ProjectService) reachable(
	ctx context.Context,
	id uuid.UUID,
	reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (*Project, error) {
	p, err := s.projects.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !reach(ctx, p.OrgID, p.ID) {
		return nil, &NotFoundError{ID: id.String()}
	}
	return p, nil
}
