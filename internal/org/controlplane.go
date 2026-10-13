package org

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/kernel/authz"
)

// OrgService implements org.v1.OrgService.
type OrgService struct {
	orgs *Service
}

// NewControlPlane binds the handler to the org service.
func NewControlPlane(orgs *Service) *OrgService {
	return &OrgService{orgs: orgs}
}

var _ orgv1connect.OrgServiceHandler = (*OrgService)(nil)

// CreateOrg creates an organization, minting a slug when one is not supplied.
func (s *OrgService) CreateOrg(
	ctx context.Context,
	req *connect.Request[orgv1.CreateOrgRequest],
) (*connect.Response[orgv1.CreateOrgResponse], error) {
	o, err := s.orgs.Create(ctx, CreateRequest{Slug: req.Msg.GetSlug(), Name: req.Msg.GetName()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&orgv1.CreateOrgResponse{Org: orgToProto(o)}), nil
}

// GetOrg loads an organization by id.
func (s *OrgService) GetOrg(
	ctx context.Context,
	req *connect.Request[orgv1.GetOrgRequest],
) (*connect.Response[orgv1.GetOrgResponse], error) {
	o, err := s.reachable(ctx, req.Msg.GetId(), authz.ReachOrg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&orgv1.GetOrgResponse{Org: orgToProto(o)}), nil
}

// ListOrgs lists the organizations the caller reaches, ordered by creation time.
func (s *OrgService) ListOrgs(
	ctx context.Context,
	_ *connect.Request[orgv1.ListOrgsRequest],
) (*connect.Response[orgv1.ListOrgsResponse], error) {
	orgs, err := s.orgs.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*orgv1.Org, 0, len(orgs))
	for _, o := range orgs {
		if authz.ReachOrg(ctx, o.ID) {
			out = append(out, orgToProto(o))
		}
	}
	return connect.NewResponse(&orgv1.ListOrgsResponse{Orgs: out}), nil
}

// UpdateOrg changes an organization's display name and slug.
func (s *OrgService) UpdateOrg(
	ctx context.Context,
	req *connect.Request[orgv1.UpdateOrgRequest],
) (*connect.Response[orgv1.UpdateOrgResponse], error) {
	o, err := s.reachable(ctx, req.Msg.GetId(), authz.ReachWholeOrg)
	if err != nil {
		return nil, err
	}
	o, err = s.orgs.Update(ctx, o.ID, req.Msg.GetName(), req.Msg.GetSlug())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&orgv1.UpdateOrgResponse{Org: orgToProto(o)}), nil
}

// SECURITY: an unknown public id and an org out of reach answer the same not-found.
func (s *OrgService) reachable(ctx context.Context, publicID string, reach func(context.Context, uuid.UUID) bool) (*Org, error) {
	o, err := s.orgs.ByPublicID(ctx, publicID)
	if IsNotFoundError(err) {
		return nil, &NotFoundError{}
	}
	if err != nil {
		return nil, err
	}
	if !reach(ctx, o.ID) {
		return nil, &NotFoundError{}
	}
	return o, nil
}
