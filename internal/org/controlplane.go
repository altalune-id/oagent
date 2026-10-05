package org

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/auth-server/apperror"
	orgv1 "altalune.id/auth-server/gen/go/org/v1"
	orgv1connect "altalune.id/auth-server/gen/go/org/v1/orgv1connect"
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
