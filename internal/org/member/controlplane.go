package member

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	memberv1 "altalune.id/oagent/gen/go/member/v1"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

// MemberService implements member.v1.MemberService; orgs and members are named by public ids.
type MemberService struct {
	members *Service
	refs    Refs
}

// NewControlPlane binds the handler to the membership service and the org and user public id lookups.
func NewControlPlane(members *Service, refs Refs) *MemberService {
	return &MemberService{members: members, refs: refs}
}

var _ memberv1connect.MemberServiceHandler = (*MemberService)(nil)

// ListMembers lists an organization's members with their email, name and role.
func (s *MemberService) ListMembers(
	ctx context.Context,
	req *connect.Request[memberv1.ListMembersRequest],
) (*connect.Response[memberv1.ListMembersResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	ms, err := s.members.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*memberv1.Member, 0, len(ms))
	for i := range ms {
		out = append(out, memberToProto(&ms[i]))
	}
	return connect.NewResponse(&memberv1.ListMembersResponse{Members: out}), nil
}

// SetMemberRole changes a member's role; only a person or the operator may.
func (s *MemberService) SetMemberRole(
	ctx context.Context,
	req *connect.Request[memberv1.SetMemberRoleRequest],
) (*connect.Response[memberv1.SetMemberRoleResponse], error) {
	orgID, userID, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	r, err := RequesterFrom(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.members.SetRole(ctx, r, orgID, userID, Role(req.Msg.GetRole()))
	if err != nil {
		return nil, err
	}
	out := &Member{Membership: *m, UserPublicID: req.Msg.GetUserId()}
	if ms, err := s.members.List(ctx, orgID); err == nil {
		for i := range ms {
			if ms[i].UserID == userID {
				out = &ms[i]
				break
			}
		}
	}
	return connect.NewResponse(&memberv1.SetMemberRoleResponse{Member: memberToProto(out)}), nil
}

// RemoveMember takes a member out of an organization; only a person or the operator may.
func (s *MemberService) RemoveMember(
	ctx context.Context,
	req *connect.Request[memberv1.RemoveMemberRequest],
) (*connect.Response[memberv1.RemoveMemberResponse], error) {
	orgID, userID, err := s.target(ctx, req.Msg.GetOrgId(), req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	r, err := RequesterFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.members.Remove(ctx, r, orgID, userID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&memberv1.RemoveMemberResponse{}), nil
}

// LeaveOrg takes the signed-in person out of an organization.
func (s *MemberService) LeaveOrg(
	ctx context.Context,
	req *connect.Request[memberv1.LeaveOrgRequest],
) (*connect.Response[memberv1.LeaveOrgResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	p := session.PrincipalFrom(ctx)
	if !p.IsPerson() {
		return nil, &authn.UnauthorizedError{}
	}
	if err := s.members.Leave(ctx, orgID, p.UserID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&memberv1.LeaveOrgResponse{}), nil
}

// SECURITY: an unknown org and an org out of reach answer the same not-found.
func (s *MemberService) org(ctx context.Context, publicID string) (uuid.UUID, error) {
	id, ok, err := s.refs.OrgID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !ok || !authz.ReachWholeOrg(ctx, id) {
		return uuid.Nil, &NotFoundError{}
	}
	return id, nil
}

// SECURITY: an unknown user answers the same not-found as a user who is not a member.
func (s *MemberService) target(ctx context.Context, orgPublicID, userPublicID string) (orgID, userID uuid.UUID, err error) {
	if orgID, err = s.org(ctx, orgPublicID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	userID, ok, err := s.refs.UserID(ctx, userPublicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if !ok {
		return uuid.Nil, uuid.Nil, &NotFoundError{}
	}
	return orgID, userID, nil
}

// RequesterFrom builds the Requester from the Principal on ctx.
func RequesterFrom(ctx context.Context) (Requester, error) {
	// SECURITY: keys never act; a person's role comes from the membership row inside the change, never from the session's scopes.
	p := session.PrincipalFrom(ctx)
	switch {
	case p.IsPerson():
		return Requester{UserID: p.UserID, Operator: p.Operator}, nil
	case p.Operator:
		return Requester{Operator: true}, nil
	}
	return Requester{}, &authz.ForbiddenError{Permission: scope.MemberWrite}
}

func memberToProto(m *Member) *memberv1.Member {
	return &memberv1.Member{
		UserId:   m.UserPublicID,
		Email:    m.Email,
		Name:     m.Name,
		Role:     string(m.Role),
		JoinedAt: timestamppb.New(m.CreatedAt),
	}
}
