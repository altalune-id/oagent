package project

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	projectv1 "altalune.id/oagent/gen/go/project/v1"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/kernel/authz"
)

// ProjectService implements project.v1.ProjectService; every id it takes and returns is a public id.
type ProjectService struct {
	projects *Service
	orgs     OrgRefs
}

// NewControlPlane binds the handler to the project service and the org public id lookup.
func NewControlPlane(projects *Service, orgs OrgRefs) *ProjectService {
	return &ProjectService{projects: projects, orgs: orgs}
}

var _ projectv1connect.ProjectServiceHandler = (*ProjectService)(nil)

// CreateProject creates a project within an organization, minting a slug when one is not supplied.
func (s *ProjectService) CreateProject(
	ctx context.Context,
	req *connect.Request[projectv1.CreateProjectRequest],
) (*connect.Response[projectv1.CreateProjectResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId(), authz.ReachWholeOrg)
	if err != nil {
		return nil, err
	}
	p, err := s.projects.Create(ctx, CreateRequest{OrgID: orgID, Slug: req.Msg.GetSlug(), Name: req.Msg.GetName()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.CreateProjectResponse{Project: projectToProto(p, req.Msg.GetOrgId())}), nil
}

// GetProject loads a project by id.
func (s *ProjectService) GetProject(
	ctx context.Context,
	req *connect.Request[projectv1.GetProjectRequest],
) (*connect.Response[projectv1.GetProjectResponse], error) {
	p, err := s.reachable(ctx, req.Msg.GetId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, p)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.GetProjectResponse{Project: out}), nil
}

// ListProjects lists the organization's projects the caller reaches, ordered by creation time.
func (s *ProjectService) ListProjects(
	ctx context.Context,
	req *connect.Request[projectv1.ListProjectsRequest],
) (*connect.Response[projectv1.ListProjectsResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId(), authz.ReachOrg)
	if err != nil {
		return nil, err
	}
	projects, err := s.projects.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*projectv1.Project, 0, len(projects))
	for _, p := range projects {
		if authz.ReachProject(ctx, p.OrgID, p.ID) {
			out = append(out, projectToProto(p, req.Msg.GetOrgId()))
		}
	}
	return connect.NewResponse(&projectv1.ListProjectsResponse{Projects: out}), nil
}

// UpdateProject renames a project and/or changes its slug.
func (s *ProjectService) UpdateProject(
	ctx context.Context,
	req *connect.Request[projectv1.UpdateProjectRequest],
) (*connect.Response[projectv1.UpdateProjectResponse], error) {
	p, err := s.reachable(ctx, req.Msg.GetId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	p, err = s.projects.Update(ctx, p.ID, req.Msg.GetName(), req.Msg.GetSlug())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, p)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&projectv1.UpdateProjectResponse{Project: out}), nil
}

// SECURITY: an unknown org and an org out of reach answer the same not-found.
func (s *ProjectService) org(ctx context.Context, publicID string, reach func(context.Context, uuid.UUID) bool) (uuid.UUID, error) {
	id, ok, err := s.orgs.OrgID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !ok || !reach(ctx, id) {
		return uuid.Nil, &NotFoundError{}
	}
	return id, nil
}

// SECURITY: an unknown project and a project out of reach answer the same not-found.
func (s *ProjectService) reachable(
	ctx context.Context,
	publicID string,
	reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (*Project, error) {
	p, err := s.projects.ByPublicID(ctx, publicID)
	if IsNotFoundError(err) {
		return nil, &NotFoundError{}
	}
	if err != nil {
		return nil, err
	}
	if !reach(ctx, p.OrgID, p.ID) {
		return nil, &NotFoundError{}
	}
	return p, nil
}

func (s *ProjectService) toProto(ctx context.Context, p *Project) (*projectv1.Project, error) {
	orgPublicID, err := s.orgs.OrgPublicID(ctx, p.OrgID)
	if err != nil {
		return nil, err
	}
	return projectToProto(p, orgPublicID), nil
}
