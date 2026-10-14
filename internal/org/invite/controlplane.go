package invite

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
)

// InviteService implements invite.v1.InviteService; every id it takes and returns is a public id.
type InviteService struct {
	invites *Service
	orgs    OrgIDs
}

// NewControlPlane binds the handler to the invite service and the org public id lookup.
func NewControlPlane(invites *Service, orgs OrgIDs) *InviteService {
	return &InviteService{invites: invites, orgs: orgs}
}

var _ invitev1connect.InviteServiceHandler = (*InviteService)(nil)

// CreateInvite invites an email and returns the link once.
func (s *InviteService) CreateInvite(
	ctx context.Context, req *connect.Request[invitev1.CreateInviteRequest],
) (*connect.Response[invitev1.CreateInviteResponse], error) {
	orgID, r, err := s.writer(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	c, err := s.invites.Create(ctx, r, orgID, req.Msg.GetEmail(), member.Role(req.Msg.GetRole()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.CreateInviteResponse{Invite: s.toProto(ctx, c.Invite), Link: c.Link}), nil
}

// ResendInvite gives an invite a new link and expiry and emails it again.
func (s *InviteService) ResendInvite(
	ctx context.Context, req *connect.Request[invitev1.ResendInviteRequest],
) (*connect.Response[invitev1.ResendInviteResponse], error) {
	orgID, inviteID, r, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetInviteId())
	if err != nil {
		return nil, err
	}
	c, err := s.invites.Resend(ctx, r, orgID, inviteID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.ResendInviteResponse{Invite: s.toProto(ctx, c.Invite), Link: c.Link}), nil
}

// RevokeInvite cancels an invite.
func (s *InviteService) RevokeInvite(
	ctx context.Context, req *connect.Request[invitev1.RevokeInviteRequest],
) (*connect.Response[invitev1.RevokeInviteResponse], error) {
	orgID, inviteID, r, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetInviteId())
	if err != nil {
		return nil, err
	}
	if err := s.invites.Revoke(ctx, r, orgID, inviteID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.RevokeInviteResponse{}), nil
}

// ListInvites lists an organization's pending invites and its drafts.
func (s *InviteService) ListInvites(
	ctx context.Context, req *connect.Request[invitev1.ListInvitesRequest],
) (*connect.Response[invitev1.ListInvitesResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	pending, err := s.invites.ListPending(ctx, orgID)
	if err != nil {
		return nil, err
	}
	drafts, err := s.invites.ListDrafts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.ListInvitesResponse{Invites: s.listToProto(pending), Drafts: s.listToProto(drafts)}), nil
}

// DraftInvite records an assistant's suggestion to invite an email as a member; a person must confirm it.
func (s *InviteService) DraftInvite(
	ctx context.Context, req *connect.Request[invitev1.DraftInviteRequest],
) (*connect.Response[invitev1.DraftInviteResponse], error) {
	orgID, r, err := s.writer(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	d, err := s.invites.Draft(ctx, r, orgID, req.Msg.GetEmail())
	if err != nil {
		return nil, err
	}
	name, err := s.invites.orgs.Name(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.DraftInviteResponse{
		Invite: s.toProto(ctx, d), OrgId: req.Msg.GetOrgId(), OrgName: strings.Join(strings.Fields(name), " "),
	}), nil
}

// ConfirmDraftInvite sends a drafted invite; its link goes only to the invitee's email.
func (s *InviteService) ConfirmDraftInvite(
	ctx context.Context, req *connect.Request[invitev1.ConfirmDraftInviteRequest],
) (*connect.Response[invitev1.ConfirmDraftInviteResponse], error) {
	orgID, inviteID, r, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetInviteId())
	if err != nil {
		return nil, err
	}
	c, err := s.invites.ConfirmDraft(ctx, r, orgID, inviteID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.ConfirmDraftInviteResponse{Invite: s.toProto(ctx, c.Invite)}), nil
}

// DiscardDraftInvite drops a draft.
func (s *InviteService) DiscardDraftInvite(
	ctx context.Context, req *connect.Request[invitev1.DiscardDraftInviteRequest],
) (*connect.Response[invitev1.DiscardDraftInviteResponse], error) {
	orgID, inviteID, r, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetInviteId())
	if err != nil {
		return nil, err
	}
	if err := s.invites.DiscardDraft(ctx, r, orgID, inviteID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&invitev1.DiscardDraftInviteResponse{}), nil
}

// SECURITY: an unknown org and one out of reach answer the same not-found.
func (s *InviteService) org(ctx context.Context, publicID string) (uuid.UUID, error) {
	id, err := s.orgs.IDByPublicID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !authz.ReachWholeOrg(ctx, id) {
		return uuid.Nil, &org.NotFoundError{}
	}
	return id, nil
}

func (s *InviteService) writer(ctx context.Context, orgPublicID string) (uuid.UUID, member.Requester, error) {
	orgID, err := s.org(ctx, orgPublicID)
	if err != nil {
		return uuid.Nil, member.Requester{}, err
	}
	r, err := member.RequesterFrom(ctx)
	if err != nil {
		return uuid.Nil, member.Requester{}, err
	}
	return orgID, r, nil
}

func (s *InviteService) target(
	ctx context.Context, orgPublicID, invitePublicID string,
) (orgID, inviteID uuid.UUID, r member.Requester, err error) {
	if orgID, r, err = s.writer(ctx, orgPublicID); err != nil {
		return uuid.Nil, uuid.Nil, member.Requester{}, err
	}
	inv, err := s.invites.ByPublicID(ctx, invitePublicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, member.Requester{}, err
	}
	if inv.OrgID != orgID {
		return uuid.Nil, uuid.Nil, member.Requester{}, &NotFoundError{}
	}
	return orgID, inv.ID, r, nil
}

func (s *InviteService) toProto(ctx context.Context, i *Invite) *invitev1.Invite {
	if i.InviterName == "" {
		i.InviterName = s.invites.inviterName(ctx, i.InvitedBy)
	}
	return &invitev1.Invite{
		Id:            i.PublicID,
		Email:         i.Email,
		Role:          string(i.Role),
		Status:        string(i.Status(s.invites.now())),
		InvitedByName: i.InviterName,
		ExpiresAt:     timestamppb.New(i.ExpiresAt),
		CreatedAt:     timestamppb.New(i.CreatedAt),
	}
}

func (s *InviteService) listToProto(list []*Invite) []*invitev1.Invite {
	out := make([]*invitev1.Invite, 0, len(list))
	for _, i := range list {
		out = append(out, s.toProto(context.Background(), i))
	}
	return out
}
