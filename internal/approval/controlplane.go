package approval

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	approvalv1 "altalune.id/oagent/gen/go/approval/v1"
	approvalv1connect "altalune.id/oagent/gen/go/approval/v1/approvalv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// ApprovalService implements approval.v1.ApprovalService; every id it takes and returns is a public id.
type ApprovalService struct {
	approvals *Service
	refs      HandlerRefs
	names     Names
}

// NewControlPlane binds the handler to the approval service, the public id lookups and the label rule.
func NewControlPlane(approvals *Service, refs HandlerRefs, names Names) *ApprovalService {
	return &ApprovalService{approvals: approvals, refs: refs, names: names}
}

var _ approvalv1connect.ApprovalServiceHandler = (*ApprovalService)(nil)

// ListApprovals lists a project's approvals the caller reaches, newest first, with the reachable pending count.
func (s *ApprovalService) ListApprovals(
	ctx context.Context,
	req *connect.Request[approvalv1.ListApprovalsRequest],
) (*connect.Response[approvalv1.ListApprovalsResponse], error) {
	m := req.Msg
	projectID, orgID, ok, err := s.refs.ProjectID(ctx, m.GetProjectId())
	if err != nil {
		return nil, err
	}
	// SECURITY: an unknown or unreachable project answers the same not-found as an unknown approval.
	if !ok || !authz.ReachProject(ctx, orgID, projectID) {
		return nil, &NotFoundError{}
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
	// SECURITY: an agent-scoped caller sees, and counts, only its own agents' approvals.
	var reach []uuid.UUID
	if !authz.ReachWholeProject(ctx, orgID, projectID) {
		reach = session.PrincipalFrom(ctx).ResourceIDs
	}
	out := &approvalv1.ListApprovalsResponse{Approvals: []*approvalv1.Approval{}}
	pending, err := s.approvals.CountPending(ctx, projectID, reach)
	if err != nil {
		return nil, err
	}
	out.PendingCount = int64(pending)
	filter := Filter{ProjectID: projectID, AgentIDs: reach, Status: Status(m.GetStatus())}
	if m.GetAgentId() != "" {
		agentID, found, err := s.refs.ResourceID(ctx, orgID, m.GetAgentId())
		if err != nil {
			return nil, err
		}
		if !found || !authz.ReachResource(ctx, orgID, projectID, agentID) {
			return connect.NewResponse(out), nil
		}
		filter.AgentIDs = []uuid.UUID{agentID}
	}
	after, err := decodeCursor(m.GetPageToken())
	if err != nil {
		return nil, err
	}
	size := pageSize(m.GetPageSize())
	rows, err := s.approvals.List(ctx, filter, after, size+1)
	if err != nil {
		return nil, err
	}
	if len(rows) > size {
		rows = rows[:size]
		last := rows[size-1]
		out.NextPageToken = encodeCursor(Cursor{At: last.CreatedAt, PublicID: last.PublicID})
	}
	page := &ApprovalService{approvals: s.approvals, refs: s.refs, names: newPageNames(s.names)}
	for _, a := range rows {
		pb, err := page.toProto(ctx, a, m.GetProjectId())
		if err != nil {
			return nil, err
		}
		out.Approvals = append(out.Approvals, pb)
	}
	return connect.NewResponse(out), nil
}

// GetApproval loads an approval the caller reaches.
func (s *ApprovalService) GetApproval(
	ctx context.Context,
	req *connect.Request[approvalv1.GetApprovalRequest],
) (*connect.Response[approvalv1.GetApprovalResponse], error) {
	ctx, ref, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.approvals.Get(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	pb, err := s.toProto(ctx, a, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&approvalv1.GetApprovalResponse{Approval: pb}), nil
}

// DecideApproval answers a pending approval through the same domain call as WhatsApp, then wakes the waiting run.
func (s *ApprovalService) DecideApproval(
	ctx context.Context,
	req *connect.Request[approvalv1.DecideApprovalRequest],
) (*connect.Response[approvalv1.DecideApprovalResponse], error) {
	m := req.Msg
	ctx, ref, err := s.reachable(ctx, m.GetId())
	if err != nil {
		return nil, err
	}
	cur, err := s.approvals.Get(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	// SECURITY: an API conversation's approval is answered only by its own actor, through the resume on the data plane.
	if cur.Channel == ChannelAPI {
		return nil, &AnsweredByAppError{}
	}
	a, err := s.approvals.Decide(ctx, ref.ID, Answer{
		Approved: m.GetApproved(), Text: m.GetText(), By: DeciderOf(ctx), Via: viaOf(ctx, req.Peer().Protocol), Kick: true,
	})
	if err != nil {
		return nil, err
	}
	pb, err := s.toProto(ctx, a, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&approvalv1.DecideApprovalResponse{Approval: pb}), nil
}

// SECURITY: unknown and out-of-reach approvals answer the same not-found; reach follows the approval's agent.
func (s *ApprovalService) reachable(ctx context.Context, publicID string) (context.Context, Ref, error) {
	ref, err := s.approvals.Resolve(ctx, publicID)
	if IsNotFoundError(err) || (err == nil && !authz.ReachResource(ctx, ref.OrgID, ref.ProjectID, ref.AgentID)) {
		return ctx, Ref{}, &NotFoundError{}
	}
	if err != nil {
		return ctx, Ref{}, err
	}
	return tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID}), ref, nil
}

// NOTE: the MCP tools call this handler in process with a bare request and no peer protocol; over Connect, the caller's kind decides.
func viaOf(ctx context.Context, peerProtocol string) Via {
	switch {
	case peerProtocol == "":
		return ViaMCP
	case session.PrincipalFrom(ctx).Source == session.SourceAPIKey:
		return ViaAPI
	}
	return ViaConsole
}

// DeciderOf is who acts for the request on ctx: the signed-in person, else the API key, else the system.
func DeciderOf(ctx context.Context) Decider {
	p := session.PrincipalFrom(ctx)
	if p.IsPerson() {
		return Decider{Kind: DeciderUser, ID: p.UserID.String()}
	}
	if p.KeyID != uuid.Nil {
		return Decider{Kind: DeciderAPIKey, ID: p.KeyID.String()}
	}
	return Decider{Kind: DeciderSystem}
}

// SECURITY: the requester and decider show as labels and public ids only; the preview is the stored plain text.
func (s *ApprovalService) toProto(ctx context.Context, a *Approval, projectPub string) (*approvalv1.Approval, error) {
	var err error
	if projectPub == "" {
		if projectPub, err = s.refs.ProjectPublicID(ctx, a.ProjectID); err != nil {
			return nil, err
		}
	}
	out := &approvalv1.Approval{
		Id: a.PublicID, ProjectId: projectPub, Status: string(a.Status), Channel: string(a.Channel), Tools: a.Tools(),
		Preview: a.Preview, DecisionText: a.DecisionText, DecidedVia: string(a.DecidedVia),
		CreatedAt: timestamppb.New(a.CreatedAt), ExpiresAt: timestamppb.New(a.ExpiresAt),
	}
	if a.DecidedAt != nil {
		out.DecidedAt = timestamppb.New(*a.DecidedAt)
	}
	if out.AgentId, _, err = s.refs.ResourcePublicID(ctx, a.OrgID, a.AgentID); err != nil {
		return nil, err
	}
	if out.ThreadId, err = s.refs.ThreadPublicID(ctx, a.ThreadID); err != nil {
		return nil, err
	}
	if out.RunId, err = s.refs.RunPublicID(ctx, a.RunID); err != nil {
		return nil, err
	}
	if a.Requester.Valid {
		if out.RequesterEndUserId, out.RequesterLabel, err = s.names.EndUser(ctx, a.Requester.UUID); err != nil {
			return nil, err
		}
	}
	if err := s.decider(ctx, a, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ApprovalService) decider(ctx context.Context, a *Approval, out *approvalv1.Approval) error {
	id, parseErr := uuid.Parse(a.DecidedByID)
	if parseErr != nil {
		return nil //nolint:nilerr // pending, or decided by the system: nobody to name.
	}
	var err error
	switch a.DecidedByKind {
	case DeciderUser:
		if out.DecidedBy, err = s.refs.UserPublicID(ctx, id); err != nil {
			return err
		}
		out.DecidedByLabel, err = s.names.Member(ctx, id)
	case DeciderAPIKey:
		var hint string
		if hint, err = s.names.KeyHint(ctx, id); hint != "" {
			out.DecidedByLabel = "…" + hint
		}
	case DeciderEndUser:
		_, out.DecidedByLabel, err = s.names.EndUser(ctx, id)
	}
	return err
}

func pageSize(n int32) int {
	if n <= 0 {
		return defaultPageSize
	}
	return min(int(n), maxPageSize)
}

func encodeCursor(c Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.At.UTC().Format(time.RFC3339Nano) + "|" + c.PublicID))
}

func decodeCursor(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	at, pub, ok := strings.Cut(string(raw), "|")
	if err != nil || !ok || pub == "" {
		return Cursor{}, &InvalidPageTokenError{}
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return Cursor{}, &InvalidPageTokenError{}
	}
	return Cursor{At: t, PublicID: pub}, nil
}

type person struct{ publicID, label string }

// NOTE: pageNames looks each person up once per page; a page often repeats the same requester or member.
type pageNames struct {
	Names
	endUsers map[uuid.UUID]person
	members  map[uuid.UUID]string
	keys     map[uuid.UUID]string
}

func newPageNames(n Names) *pageNames {
	return &pageNames{Names: n, endUsers: map[uuid.UUID]person{}, members: map[uuid.UUID]string{}, keys: map[uuid.UUID]string{}}
}

func (p *pageNames) EndUser(ctx context.Context, id uuid.UUID) (publicID, label string, err error) {
	if got, ok := p.endUsers[id]; ok {
		return got.publicID, got.label, nil
	}
	publicID, label, err = p.Names.EndUser(ctx, id)
	if err == nil {
		p.endUsers[id] = person{publicID: publicID, label: label}
	}
	return publicID, label, err
}

func (p *pageNames) Member(ctx context.Context, userID uuid.UUID) (string, error) {
	if got, ok := p.members[userID]; ok {
		return got, nil
	}
	name, err := p.Names.Member(ctx, userID)
	if err == nil {
		p.members[userID] = name
	}
	return name, err
}

func (p *pageNames) KeyHint(ctx context.Context, keyID uuid.UUID) (string, error) {
	if got, ok := p.keys[keyID]; ok {
		return got, nil
	}
	hint, err := p.Names.KeyHint(ctx, keyID)
	if err == nil {
		p.keys[keyID] = hint
	}
	return hint, err
}
