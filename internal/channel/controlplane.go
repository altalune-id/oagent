package channel

import (
	"context"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	channelv1 "altalune.id/oagent/gen/go/channel/v1"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ControlPlaneOptions are the server settings the channel API reports: delivery URL, default OpenWA URL, message age limit.
type ControlPlaneOptions struct {
	// HooksURL is the absolute URL OpenWA posts to, without the connection id; empty leaves webhook_url empty.
	HooksURL       string
	DefaultBaseURL string
	MaxMessageAge  time.Duration
}

// ChannelService implements channel.v1.ChannelService; every id it takes and returns is a public id.
type ChannelService struct {
	conns *Service
	refs  Refs
	opts  ControlPlaneOptions
}

// NewControlPlane binds the handler to the connection service, the public id lookups and the server settings.
func NewControlPlane(conns *Service, refs Refs, opts ControlPlaneOptions) *ChannelService {
	return &ChannelService{conns: conns, refs: refs, opts: opts}
}

var _ channelv1connect.ChannelServiceHandler = (*ChannelService)(nil)

// CreateConnection binds a project agent to an OpenWA device and seals the device key; SetConnectionSecret adds the secret.
func (s *ChannelService) CreateConnection(
	ctx context.Context,
	req *connect.Request[channelv1.CreateConnectionRequest],
) (*connect.Response[channelv1.CreateConnectionResponse], error) {
	m := req.Msg
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	agentID, err := s.agent(ctx, projectID, m.GetAgentId())
	if err != nil {
		return nil, err
	}
	// SECURITY: checked after reach, so a caller outside the project learns nothing about this server.
	if s.opts.HooksURL == "" {
		return nil, &WebhookURLUnavailableError{}
	}
	in := Input{
		BaseURL: m.GetBaseUrl(), OpenWAOrg: m.GetOpenwaOrg(), OpenWAProject: m.GetOpenwaProject(), DeviceID: m.GetDeviceId(),
		QuietWindow: millis(m.QuietWindowMs), QuietMax: millis(m.QuietMaxMs),
		ReplyFailure: m.GetReplyFailure(), ReplyVoice: m.GetReplyVoice(), ReplyPhotoUnreadable: m.GetReplyPhotoUnreadable(),
	}
	c, err := s.conns.Create(ctx, projectID, agentID, in, m.GetDeviceKey())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, c)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.CreateConnectionResponse{Connection: out}), nil
}

// GetConnection loads a connection by id.
func (s *ChannelService) GetConnection(
	ctx context.Context,
	req *connect.Request[channelv1.GetConnectionRequest],
) (*connect.Response[channelv1.GetConnectionResponse], error) {
	ctx, c, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, c)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.GetConnectionResponse{Connection: out}), nil
}

// ListConnections lists the project's connections the caller reaches, oldest first, optionally for one agent.
func (s *ChannelService) ListConnections(
	ctx context.Context,
	req *connect.Request[channelv1.ListConnectionsRequest],
) (*connect.Response[channelv1.ListConnectionsResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	agentID := uuid.Nil
	if req.Msg.GetAgentId() != "" {
		if agentID, err = s.agent(ctx, projectID, req.Msg.GetAgentId()); err != nil {
			return nil, err
		}
	}
	conns, err := s.conns.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*channelv1.Connection, 0, len(conns))
	for _, c := range conns {
		if (agentID != uuid.Nil && c.AgentID != agentID) || !authz.ReachResource(ctx, c.OrgID, c.ProjectID, c.AgentID) {
			continue
		}
		pc, err := s.toProto(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return connect.NewResponse(&channelv1.ListConnectionsResponse{Connections: out}), nil
}

// UpdateConnection replaces a connection's settings; an unset device key, quiet time or switch keeps the stored value.
func (s *ChannelService) UpdateConnection(
	ctx context.Context,
	req *connect.Request[channelv1.UpdateConnectionRequest],
) (*connect.Response[channelv1.UpdateConnectionResponse], error) {
	m := req.Msg
	ctx, c, err := s.reachable(ctx, m.GetId())
	if err != nil {
		return nil, err
	}
	in := Input{
		BaseURL: keep(m.BaseUrl, c.BaseURL), OpenWAOrg: keep(m.OpenwaOrg, c.OpenWAOrg), OpenWAProject: keep(m.OpenwaProject, c.OpenWAProject),
		DeviceID: keep(m.DeviceId, c.DeviceID), QuietWindow: millis(m.QuietWindowMs), QuietMax: millis(m.QuietMaxMs),
		ReplyFailure: m.GetReplyFailure(), ReplyVoice: m.GetReplyVoice(), ReplyPhotoUnreadable: m.GetReplyPhotoUnreadable(),
		ApprovalPrompt: m.ApprovalPrompt, ApprovalExpired: m.ApprovalExpired,
	}
	if m.Enabled != nil {
		in.Status = StatusDisabled
		if m.GetEnabled() {
			in.Status = StatusActive
		}
	}
	c, err = s.conns.Update(ctx, c.ID, in, m.DeviceKey)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, c)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.UpdateConnectionResponse{Connection: out}), nil
}

// SetConnectionSecret makes the signing secret OpenWA minted primary; the old one stays valid unless drop_previous.
func (s *ChannelService) SetConnectionSecret(
	ctx context.Context,
	req *connect.Request[channelv1.SetConnectionSecretRequest],
) (*connect.Response[channelv1.SetConnectionSecretResponse], error) {
	ctx, c, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if c, err = s.conns.SetSecret(ctx, c.ID, req.Msg.GetSigningSecret(), req.Msg.GetDropPrevious()); err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, c)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.SetConnectionSecretResponse{Connection: out}), nil
}

// DeleteConnection deletes a connection; its threads and messages are kept.
func (s *ChannelService) DeleteConnection(
	ctx context.Context,
	req *connect.Request[channelv1.DeleteConnectionRequest],
) (*connect.Response[channelv1.DeleteConnectionResponse], error) {
	ctx, c, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.conns.Delete(ctx, c.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&channelv1.DeleteConnectionResponse{}), nil
}

// GetChannelSettings reports what a new connection starts with.
func (s *ChannelService) GetChannelSettings(
	context.Context, *connect.Request[channelv1.GetChannelSettingsRequest],
) (*connect.Response[channelv1.GetChannelSettingsResponse], error) {
	return connect.NewResponse(&channelv1.GetChannelSettingsResponse{
		DefaultOpenwaBaseUrl: s.opts.DefaultBaseURL,
		ReplyFailure:         DefaultReplyFailure, ReplyVoice: DefaultReplyVoice, ReplyPhotoUnreadable: DefaultReplyPhotoUnreadable,
		QuietWindowMs: ms(defaultQuietWindow), QuietMaxMs: ms(defaultQuietMax),
		MaxMessageAgeMinutes: int32(min(s.opts.MaxMessageAge.Minutes(), math.MaxInt32)),
		ApprovalPrompt:       DefaultApprovalPrompt, ApprovalExpired: DefaultApprovalExpired,
	}), nil
}

// SECURITY: unknown and out-of-reach connections answer the same not-found; a reached connection scopes the call to its org.
func (s *ChannelService) reachable(ctx context.Context, publicID string) (context.Context, *Connection, error) {
	c, err := s.conns.Resolve(ctx, publicID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	if err != nil {
		return ctx, nil, err
	}
	if !authz.ReachResource(ctx, c.OrgID, c.ProjectID, c.AgentID) {
		return ctx, nil, &NotFoundError{}
	}
	return tenant.WithOrg(ctx, c.OrgID), c, nil
}

// SECURITY: an unknown or unreachable project is reported as an absent connection; a reached project scopes the call to its org.
func (s *ChannelService) project(
	ctx context.Context, publicID string, reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (context.Context, uuid.UUID, error) {
	id, orgID, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !reach(ctx, orgID, id) {
		return ctx, uuid.Nil, &NotFoundError{}
	}
	return tenant.WithOrg(ctx, orgID), id, nil
}

// SECURITY: an agent of another org, or one the caller doesn't reach, is reported as an absent connection.
func (s *ChannelService) agent(ctx context.Context, projectID uuid.UUID, publicID string) (uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	id, ok, err := s.refs.ResourceID(ctx, tc.OrgID, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !ok || !authz.ReachResource(ctx, tc.OrgID, projectID, id) {
		return uuid.Nil, &NotFoundError{}
	}
	return id, nil
}

// SECURITY: sealed secrets never leave the server; the wire carries only whether a secret, and a previous one, are set.
func (s *ChannelService) toProto(ctx context.Context, c *Connection) (*channelv1.Connection, error) {
	orgPub, err := s.refs.OrgPublicID(ctx, c.OrgID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, c.ProjectID)
	if err != nil {
		return nil, err
	}
	agentPub, _, err := s.refs.ResourcePublicID(ctx, c.OrgID, c.AgentID)
	if err != nil {
		return nil, err
	}
	out := &channelv1.Connection{
		Id: c.PublicID, OrgId: orgPub, ProjectId: projectPub, AgentId: agentPub, Provider: c.Provider,
		Enabled: c.Status == StatusActive, Status: statusToProto(c.ReportedStatus()),
		BaseUrl: c.BaseURL, OpenwaOrg: c.OpenWAOrg, OpenwaProject: c.OpenWAProject, DeviceId: c.DeviceID,
		SecretSet: len(c.SigningPrimarySealed) > 0, SecondarySet: len(c.SigningSecondarySealed) > 0,
		QuietWindowMs: ms(c.QuietWindow), QuietMaxMs: ms(c.QuietMax),
		ReplyFailure: c.ReplyFailure, ReplyVoice: c.ReplyVoice, ReplyPhotoUnreadable: c.ReplyPhotoUnreadable,
		ApprovalPrompt: c.ApprovalPrompt, ApprovalExpired: c.ApprovalExpired,
		LastResultAt: stamp(c.LastResultAt), LastBadSignatureAt: stamp(c.LastBadSignatureAt),
		CreatedAt: timestamppb.New(c.CreatedAt), UpdatedAt: timestamppb.New(c.UpdatedAt),
	}
	if s.opts.HooksURL != "" {
		out.WebhookUrl = s.opts.HooksURL + "/" + c.PublicID
	}
	return out, nil
}

func statusToProto(reported string) channelv1.ConnectionStatus {
	switch reported {
	case ResultConnected:
		return channelv1.ConnectionStatus_CONNECTION_STATUS_CONNECTED
	case ResultDeviceMismatch:
		return channelv1.ConnectionStatus_CONNECTION_STATUS_DEVICE_MISMATCH
	case ResultKeyRefused:
		return channelv1.ConnectionStatus_CONNECTION_STATUS_KEY_REFUSED
	default:
		return channelv1.ConnectionStatus_CONNECTION_STATUS_WAITING
	}
}

func keep(set *string, stored string) string {
	if set == nil {
		return stored
	}
	return *set
}

func millis(ms *int32) *time.Duration {
	if ms == nil {
		return nil
	}
	d := time.Duration(*ms) * time.Millisecond
	return &d
}

func stamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
