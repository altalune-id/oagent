package project

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
)

// ProjectService implements project.v1.ProjectService.
type ProjectService struct {
	projects *Service
}

// NewProjectService binds the handler to the project service.
func NewProjectService(projects *Service) *ProjectService {
	return &ProjectService{projects: projects}
}

var _ projectv1connect.ProjectServiceHandler = (*ProjectService)(nil)

// CreateProject creates a project within an organization, minting a slug when one is not supplied.
func (s *ProjectService) CreateProject(ctx context.Context, req *connect.Request[projectv1.CreateProjectRequest]) (*connect.Response[projectv1.CreateProjectResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	p, err := s.projects.Create(ctx, CreateRequest{OrgID: orgID, Slug: req.Msg.GetSlug(), Name: req.Msg.GetName()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.CreateProjectResponse{Project: projectToProto(p)}), nil
}

// GetProject loads a project by id.
func (s *ProjectService) GetProject(ctx context.Context, req *connect.Request[projectv1.GetProjectRequest]) (*connect.Response[projectv1.GetProjectResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	p, err := s.projects.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.GetProjectResponse{Project: projectToProto(p)}), nil
}

// ListProjects lists an organization's projects, ordered by creation time.
func (s *ProjectService) ListProjects(ctx context.Context, req *connect.Request[projectv1.ListProjectsRequest]) (*connect.Response[projectv1.ListProjectsResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	projects, err := s.projects.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*projectv1.Project, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectToProto(p))
	}
	return connect.NewResponse(&projectv1.ListProjectsResponse{Projects: out}), nil
}
