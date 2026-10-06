package org

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
)

// OrgService implements org.v1.OrgService.
type OrgService struct {
	orgs *Service
}

// NewOrgService binds the handler to the org service.
func NewOrgService(orgs *Service) *OrgService {
	return &OrgService{orgs: orgs}
}

var _ orgv1connect.OrgServiceHandler = (*OrgService)(nil)

// CreateOrg creates an organization, minting a slug when one is not supplied.
func (s *OrgService) CreateOrg(ctx context.Context, req *connect.Request[orgv1.CreateOrgRequest]) (*connect.Response[orgv1.CreateOrgResponse], error) {
	o, err := s.orgs.Create(ctx, CreateRequest{Slug: req.Msg.GetSlug(), Name: req.Msg.GetName()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&orgv1.CreateOrgResponse{Org: orgToProto(o)}), nil
}

// GetOrg loads an organization by id.
func (s *OrgService) GetOrg(ctx context.Context, req *connect.Request[orgv1.GetOrgRequest]) (*connect.Response[orgv1.GetOrgResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	o, err := s.orgs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&orgv1.GetOrgResponse{Org: orgToProto(o)}), nil
}

// ListOrgs lists organizations, ordered by creation time.
func (s *OrgService) ListOrgs(ctx context.Context, _ *connect.Request[orgv1.ListOrgsRequest]) (*connect.Response[orgv1.ListOrgsResponse], error) {
	orgs, err := s.orgs.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*orgv1.Org, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, orgToProto(o))
	}
	return connect.NewResponse(&orgv1.ListOrgsResponse{Orgs: out}), nil
}
