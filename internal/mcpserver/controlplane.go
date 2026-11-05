package mcpserver

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	mcpserverv1 "altalune.id/oagent/gen/go/mcpserver/v1"
	mcpserverv1connect "altalune.id/oagent/gen/go/mcpserver/v1/mcpserverv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

// McpServerService implements mcpserver.v1.McpServerService; every id it takes and returns is a public id.
type McpServerService struct {
	servers *Service
	refs    Refs
}

// NewControlPlane binds the handler to the server service and the org and project public id lookups.
func NewControlPlane(servers *Service, refs Refs) *McpServerService {
	return &McpServerService{servers: servers, refs: refs}
}

var _ mcpserverv1connect.McpServerServiceHandler = (*McpServerService)(nil)

// CreateMcpServer creates a server in a project and seals its credential.
func (s *McpServerService) CreateMcpServer(
	ctx context.Context,
	req *connect.Request[mcpserverv1.CreateMcpServerRequest],
) (*connect.Response[mcpserverv1.CreateMcpServerResponse], error) {
	m := req.Msg
	orgID, ok, err := s.refs.OrgID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	// SECURITY: a project of another org answers the same not-found as an unknown one.
	if tc, terr := tenant.From(ctx); !ok || terr != nil || tc.OrgID != orgID {
		return nil, &NotFoundError{}
	}
	in := Input{
		Name: m.GetName(), URL: m.GetUrl(), AuthKind: kindFromProto(m.GetAuthKind()), AuthHeader: m.GetAuthHeader(),
		TimeoutMS: m.GetTimeoutMs(), Trusted: m.GetTrusted(),
	}
	srv, err := s.servers.Create(ctx, projectID, in, m.GetAuthValue())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mcpserverv1.CreateMcpServerResponse{McpServer: serverToProto(srv, m.GetOrgId(), m.GetProjectId())}), nil
}

// GetMcpServer loads a server by id.
func (s *McpServerService) GetMcpServer(
	ctx context.Context,
	req *connect.Request[mcpserverv1.GetMcpServerRequest],
) (*connect.Response[mcpserverv1.GetMcpServerResponse], error) {
	ctx, srv, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, srv)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mcpserverv1.GetMcpServerResponse{McpServer: out}), nil
}

// ListMcpServers lists the project's servers the caller reaches, oldest first.
func (s *McpServerService) ListMcpServers(
	ctx context.Context,
	req *connect.Request[mcpserverv1.ListMcpServersRequest],
) (*connect.Response[mcpserverv1.ListMcpServersResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	srvs, err := s.servers.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*mcpserverv1.McpServer, 0, len(srvs))
	orgPublicID := ""
	for _, srv := range srvs {
		if !authz.ReachResource(ctx, srv.OrgID, srv.ProjectID, srv.ID) {
			continue
		}
		if orgPublicID == "" {
			if orgPublicID, err = s.refs.OrgPublicID(ctx, srv.OrgID); err != nil {
				return nil, err
			}
		}
		out = append(out, serverToProto(srv, orgPublicID, req.Msg.GetProjectId()))
	}
	return connect.NewResponse(&mcpserverv1.ListMcpServersResponse{McpServers: out}), nil
}

// UpdateMcpServer replaces a server's editable fields; the name never changes, and an unset auth_value keeps the credential.
func (s *McpServerService) UpdateMcpServer(
	ctx context.Context,
	req *connect.Request[mcpserverv1.UpdateMcpServerRequest],
) (*connect.Response[mcpserverv1.UpdateMcpServerResponse], error) {
	m := req.Msg
	ctx, srv, err := s.reachable(ctx, m.GetId())
	if err != nil {
		return nil, err
	}
	in := Input{
		URL: m.GetUrl(), AuthKind: kindFromProto(m.GetAuthKind()), AuthHeader: m.GetAuthHeader(),
		TimeoutMS: m.GetTimeoutMs(), Trusted: m.GetTrusted(),
	}
	srv, err = s.servers.Update(ctx, srv.ID, in, m.AuthValue)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, srv)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mcpserverv1.UpdateMcpServerResponse{McpServer: out}), nil
}

// DeleteMcpServer deletes a server no published agent version uses.
func (s *McpServerService) DeleteMcpServer(
	ctx context.Context,
	req *connect.Request[mcpserverv1.DeleteMcpServerRequest],
) (*connect.Response[mcpserverv1.DeleteMcpServerResponse], error) {
	ctx, srv, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.servers.Delete(ctx, srv.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&mcpserverv1.DeleteMcpServerResponse{}), nil
}

// ListMcpServerTools connects to a server and lists its tools, suggesting approval for every tool that is not read-only.
func (s *McpServerService) ListMcpServerTools(
	ctx context.Context,
	req *connect.Request[mcpserverv1.ListMcpServerToolsRequest],
) (*connect.Response[mcpserverv1.ListMcpServerToolsResponse], error) {
	ctx, srv, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	tools, err := s.servers.ListTools(ctx, srv.ID)
	if err != nil {
		return nil, err
	}
	out := make([]*mcpserverv1.McpServerTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, &mcpserverv1.McpServerTool{
			Name: t.Name, Description: t.Description, ReadOnly: t.ReadOnly, Grantable: t.Grantable,
			RequireApproval: !t.ReadOnly, InputSchema: schemaToProto(t.InputSchema),
		})
	}
	return connect.NewResponse(&mcpserverv1.ListMcpServerToolsResponse{Tools: out}), nil
}

// SECURITY: unknown and out-of-reach servers answer the same not-found; a reached server scopes the call to its org.
func (s *McpServerService) reachable(ctx context.Context, publicID string) (context.Context, *Server, error) {
	r, err := s.servers.Resolve(ctx, publicID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	if err != nil {
		return ctx, nil, err
	}
	if !authz.ReachResource(ctx, r.OrgID, r.ProjectID, r.ID) {
		return ctx, nil, &NotFoundError{}
	}
	ctx = tenant.WithOrg(ctx, r.OrgID)
	srv, err := s.servers.Get(ctx, r.ID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	return ctx, srv, err
}

// SECURITY: an unknown or unreachable project is reported as an absent server; a reached project scopes the call to its org.
func (s *McpServerService) project(
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

func (s *McpServerService) toProto(ctx context.Context, srv *Server) (*mcpserverv1.McpServer, error) {
	orgPub, err := s.refs.OrgPublicID(ctx, srv.OrgID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, srv.ProjectID)
	if err != nil {
		return nil, err
	}
	return serverToProto(srv, orgPub, projectPub), nil
}

func kindFromProto(k mcpserverv1.McpServerAuthKind) AuthKind {
	switch k {
	case mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE:
		return AuthNone
	case mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_BEARER:
		return AuthBearer
	case mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_HEADER:
		return AuthHeader
	default:
		return ""
	}
}

func kindToProto(k AuthKind) mcpserverv1.McpServerAuthKind {
	switch k {
	case AuthNone:
		return mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE
	case AuthBearer:
		return mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_BEARER
	case AuthHeader:
		return mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_HEADER
	default:
		return mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_UNSPECIFIED
	}
}

func schemaToProto(raw json.RawMessage) *structpb.Struct {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	out, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return out
}

// SECURITY: the sealed credential never leaves the server; the wire carries only whether one is set.
func serverToProto(srv *Server, orgPublicID, projectPublicID string) *mcpserverv1.McpServer {
	return &mcpserverv1.McpServer{
		Id:         srv.PublicID,
		OrgId:      orgPublicID,
		ProjectId:  projectPublicID,
		Name:       srv.Name,
		Url:        srv.URL,
		AuthKind:   kindToProto(srv.AuthKind),
		AuthHeader: srv.AuthHeader,
		AuthSet:    len(srv.AuthSealed) > 0,
		TimeoutMs:  srv.TimeoutMS,
		Trusted:    srv.Trusted,
		CreatedAt:  timestamppb.New(srv.CreatedAt),
		UpdatedAt:  timestamppb.New(srv.UpdatedAt),
	}
}
