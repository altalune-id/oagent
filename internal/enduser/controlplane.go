package enduser

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
	enduserv1connect "altalune.id/oagent/gen/go/enduser/v1/enduserv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// EndUserService implements enduser.v1.EndUserService; every id it takes and returns is a public id.
type EndUserService struct {
	users    *Service
	refs     Refs
	activity Activity
}

// NewControlPlane binds the handler to the end-user service, the public id lookups and the conversation reads.
func NewControlPlane(users *Service, refs Refs, activity Activity) *EndUserService {
	return &EndUserService{users: users, refs: refs, activity: activity}
}

var _ enduserv1connect.EndUserServiceHandler = (*EndUserService)(nil)

// ListEndUsers lists a project's end users newest first.
func (s *EndUserService) ListEndUsers(
	ctx context.Context,
	req *connect.Request[enduserv1.ListEndUsersRequest],
) (*connect.Response[enduserv1.ListEndUsersResponse], error) {
	m := req.Msg
	projectID, orgID, ok, err := s.refs.ProjectID(ctx, m.GetProjectId())
	if err != nil {
		return nil, err
	}
	// SECURITY: people span every agent of a project, so only a caller holding the whole project reaches them.
	if !ok || !authz.ReachWholeProject(ctx, orgID, projectID) {
		return nil, &NotFoundError{ID: m.GetProjectId()}
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
	after, err := decodeCursor(m.GetPageToken())
	if err != nil {
		return nil, err
	}
	size := pageSize(m.GetPageSize())
	users, err := s.users.List(ctx, projectID, m.GetQuery(), after, size+1)
	if err != nil {
		return nil, err
	}
	out := &enduserv1.ListEndUsersResponse{EndUsers: make([]*enduserv1.EndUser, 0, min(len(users), size))}
	if len(users) > size {
		users = users[:size]
		last := users[size-1]
		out.NextPageToken = encodeCursor(Cursor{At: last.CreatedAt, PublicID: last.PublicID})
	}
	if out.EndUsers, err = s.toProto(ctx, users, m.GetProjectId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// GetEndUser loads an end user by id, with the threads they own or wrote in.
func (s *EndUserService) GetEndUser(
	ctx context.Context,
	req *connect.Request[enduserv1.GetEndUserRequest],
) (*connect.Response[enduserv1.GetEndUserResponse], error) {
	ref, err := s.users.Resolve(ctx, req.Msg.GetId())
	// SECURITY: unknown and out-of-reach end users answer the same not-found; reach needs the whole project.
	if IsNotFoundError(err) || (err == nil && !authz.ReachWholeProject(ctx, ref.OrgID, ref.ProjectID)) {
		return nil, &NotFoundError{ID: req.Msg.GetId()}
	}
	if err != nil {
		return nil, err
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID})
	e, err := s.users.Get(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, ref.ProjectID)
	if err != nil {
		return nil, err
	}
	people, err := s.toProto(ctx, []*EndUser{e}, projectPub)
	if err != nil {
		return nil, err
	}
	threads, err := s.activity.Threads(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := &enduserv1.GetEndUserResponse{EndUser: people[0], Threads: make([]*enduserv1.EndUserThread, 0, len(threads))}
	for _, th := range threads {
		agentPub, _, err := s.refs.ResourcePublicID(ctx, th.OrgID, th.AgentID)
		if err != nil {
			return nil, err
		}
		out.Threads = append(out.Threads, &enduserv1.EndUserThread{
			Id: th.PublicID, AgentId: agentPub, Kind: th.Kind, LastActivityAt: stamp(th.LastActivity),
		})
	}
	return connect.NewResponse(out), nil
}

// SECURITY: a person shows as a display name and a kind with a masked hint; no identity value leaves the server.
func (s *EndUserService) toProto(ctx context.Context, users []*EndUser, projectPub string) ([]*enduserv1.EndUser, error) {
	names, err := s.users.Names(ctx, users)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(users))
	for i, e := range users {
		ids[i] = e.ID
	}
	seen, err := s.activity.Seen(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*enduserv1.EndUser, len(users))
	for i, e := range users {
		n, a := names[e.ID], seen[e.ID]
		last := a.Last
		if last.IsZero() {
			last = e.CreatedAt
		}
		out[i] = &enduserv1.EndUser{
			Id: e.PublicID, ProjectId: projectPub, DisplayName: n.DisplayName, IdentityKind: KindProto(n.Kind), IdentityHint: n.Hint,
			FirstSeenAt: timestamppb.New(e.CreatedAt), LastSeenAt: timestamppb.New(last), ThreadCount: a.Threads,
		}
	}
	return out, nil
}

func stamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
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
