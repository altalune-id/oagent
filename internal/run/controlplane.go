package run

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	threadv1 "altalune.id/oagent/gen/go/thread/v1"
	threadv1connect "altalune.id/oagent/gen/go/thread/v1/threadv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200

	versionPublished = "published"
	versionDraft     = "draft"
)

// ThreadService implements thread.v1.ThreadService; every id it takes and returns is a public id.
type ThreadService struct {
	runs   *Service
	refs   Refs
	people People
}

// NewControlPlane binds the handler to the run service, the project and agent public id lookups and the end user names.
func NewControlPlane(runs *Service, refs Refs, people People) *ThreadService {
	return &ThreadService{runs: runs, refs: refs, people: people}
}

var _ threadv1connect.ThreadServiceHandler = (*ThreadService)(nil)

// ListThreads lists the project's threads the caller reaches, newest activity first.
func (s *ThreadService) ListThreads(
	ctx context.Context,
	req *connect.Request[threadv1.ListThreadsRequest],
) (*connect.Response[threadv1.ListThreadsResponse], error) {
	m := req.Msg
	projectID, orgID, ok, err := s.refs.ProjectID(ctx, m.GetProjectId())
	if err != nil {
		return nil, err
	}
	// SECURITY: an unknown or unreachable project answers the same not-found as an unknown thread.
	if !ok || !authz.ReachProject(ctx, orgID, projectID) {
		return nil, &ThreadNotFoundError{}
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
	out := &threadv1.ListThreadsResponse{Threads: []*threadv1.Thread{}}
	filter := ThreadFilter{Kind: m.GetKind()}
	if m.GetAgentId() != "" {
		id, found, err := s.refs.ResourceID(ctx, orgID, m.GetAgentId())
		if err != nil {
			return nil, err
		}
		if !found {
			return connect.NewResponse(out), nil
		}
		filter.AgentID = id
	}
	after, err := decodeCursor(m.GetPageToken())
	if err != nil {
		return nil, err
	}
	size := pageSize(m.GetPageSize())
	rows, err := s.runs.ListThreads(ctx, projectID, filter, after, size+1)
	if err != nil {
		return nil, err
	}
	if len(rows) > size {
		rows = rows[:size]
		last := rows[size-1]
		out.NextPageToken = encodeCursor(Cursor{At: last.LastActivityAt, PublicID: last.PublicID})
	}
	for i := range rows {
		if !authz.ReachResource(ctx, rows[i].OrgID, rows[i].ProjectID, rows[i].AgentID) {
			continue
		}
		th, err := s.threadProto(ctx, &rows[i], m.GetProjectId())
		if err != nil {
			return nil, err
		}
		out.Threads = append(out.Threads, th)
	}
	return connect.NewResponse(out), nil
}

// GetThread loads a thread by id, with its message and run counts.
func (s *ThreadService) GetThread(
	ctx context.Context,
	req *connect.Request[threadv1.GetThreadRequest],
) (*connect.Response[threadv1.GetThreadResponse], error) {
	ctx, ref, err := s.reachableThread(ctx, req.Msg.GetThreadId())
	if err != nil {
		return nil, err
	}
	sum, err := s.runs.ThreadSummary(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, ref.ProjectID)
	if err != nil {
		return nil, err
	}
	th, err := s.threadProto(ctx, sum, projectPub)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&threadv1.GetThreadResponse{Thread: th}), nil
}

// ListThreadMessages lists a thread's messages after after_seq, oldest first.
func (s *ThreadService) ListThreadMessages(
	ctx context.Context,
	req *connect.Request[threadv1.ListThreadMessagesRequest],
) (*connect.Response[threadv1.ListThreadMessagesResponse], error) {
	m := req.Msg
	ctx, ref, err := s.reachableThread(ctx, m.GetThreadId())
	if err != nil {
		return nil, err
	}
	msgs, err := s.runs.ThreadMessages(ctx, ref.ID, m.GetAfterSeq(), pageSize(m.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := &threadv1.ListThreadMessagesResponse{Messages: make([]*threadv1.ThreadMessage, 0, len(msgs)), NextAfterSeq: m.GetAfterSeq()}
	runPubs, people := map[uuid.UUID]string{}, map[uuid.UUID]Person{}
	for _, msg := range msgs {
		tm := &threadv1.ThreadMessage{
			Seq: msg.Seq, Role: string(msg.Role), Message: bodyProto(msg.AGUIBody()), Status: string(msg.Status),
			ToolCallId: msg.ToolCallID, CreatedAt: timestamppb.New(msg.CreatedAt), SkipReason: msg.SkipReason,
		}
		if msg.RunID != nil {
			if tm.RunId, err = s.runPublicID(ctx, runPubs, *msg.RunID); err != nil {
				return nil, err
			}
		}
		if msg.SpeakerID != nil {
			p, err := s.person(ctx, people, *msg.SpeakerID)
			if err != nil {
				return nil, err
			}
			tm.SpeakerLabel, tm.SpeakerDisplayName, tm.SpeakerIdentityKind, tm.SpeakerIdentityHint = p.Label, p.DisplayName, p.Kind, p.Hint
		}
		out.Messages = append(out.Messages, tm)
		out.NextAfterSeq = msg.Seq
	}
	return connect.NewResponse(out), nil
}

// ListRuns lists a thread's runs, or an agent's runs across its threads, newest first.
func (s *ThreadService) ListRuns(
	ctx context.Context,
	req *connect.Request[threadv1.ListRunsRequest],
) (*connect.Response[threadv1.ListRunsResponse], error) {
	m := req.Msg
	if m.GetThreadId() == "" {
		return s.listAgentRuns(ctx, m)
	}
	ctx, ref, err := s.reachableThread(ctx, m.GetThreadId())
	if err != nil {
		return nil, err
	}
	after, err := decodeCursor(m.GetPageToken())
	if err != nil {
		return nil, err
	}
	size := pageSize(m.GetPageSize())
	runs, err := s.runs.ListRuns(ctx, ref.ID, after, size+1)
	if err != nil {
		return nil, err
	}
	out := &threadv1.ListRunsResponse{Runs: make([]*threadv1.Run, 0, min(len(runs), size))}
	if len(runs) > size {
		runs = runs[:size]
		last := runs[size-1]
		out.NextPageToken = encodeCursor(Cursor{At: last.StartedAt, PublicID: last.PublicID})
	}
	threadPub := m.GetThreadId()
	for i := range runs {
		r, err := s.runProto(ctx, &runs[i], threadPub)
		if err != nil {
			return nil, err
		}
		out.Runs = append(out.Runs, r)
	}
	return connect.NewResponse(out), nil
}

func (s *ThreadService) listAgentRuns(
	ctx context.Context, m *threadv1.ListRunsRequest,
) (*connect.Response[threadv1.ListRunsResponse], error) {
	projectID, orgID, ok, err := s.refs.ProjectID(ctx, m.GetProjectId())
	if err != nil {
		return nil, err
	}
	agentID := uuid.Nil
	if ok {
		if agentID, ok, err = s.refs.ResourceID(ctx, orgID, m.GetAgentId()); err != nil {
			return nil, err
		}
	}
	// SECURITY: an unknown or unreachable project or agent answers the same not-found as an unknown thread.
	if !ok || !authz.ReachResource(ctx, orgID, projectID, agentID) {
		return nil, &ThreadNotFoundError{}
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
	after, err := decodeCursor(m.GetPageToken())
	if err != nil {
		return nil, err
	}
	size := pageSize(m.GetPageSize())
	runs, err := s.runs.ListAgentRuns(ctx, agentID, after, size+1)
	if err != nil {
		return nil, err
	}
	out := &threadv1.ListRunsResponse{Runs: make([]*threadv1.Run, 0, min(len(runs), size))}
	if len(runs) > size {
		runs = runs[:size]
		last := runs[size-1]
		out.NextPageToken = encodeCursor(Cursor{At: last.StartedAt, PublicID: last.PublicID})
	}
	threads := map[uuid.UUID]string{}
	for i := range runs {
		pub, ok := threads[runs[i].ThreadID]
		if !ok {
			th, err := s.runs.store.Thread(ctx, runs[i].ThreadID)
			if err != nil {
				return nil, s.runs.errOrReport(ctx, "run: thread of run", err)
			}
			pub, threads[runs[i].ThreadID] = th.PublicID, th.PublicID
		}
		r, err := s.runProto(ctx, &runs[i], pub)
		if err != nil {
			return nil, err
		}
		out.Runs = append(out.Runs, r)
	}
	return connect.NewResponse(out), nil
}

// GetRun loads a run by id.
func (s *ThreadService) GetRun(
	ctx context.Context,
	req *connect.Request[threadv1.GetRunRequest],
) (*connect.Response[threadv1.GetRunResponse], error) {
	ctx, r, err := s.reachableRun(ctx, req.Msg.GetRunId())
	if err != nil {
		return nil, err
	}
	out, err := s.runWithThread(ctx, r)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&threadv1.GetRunResponse{Run: out}), nil
}

// CancelRun cancels a run the way the data plane does: a live run stops at its next step, an ended one is returned as it is.
func (s *ThreadService) CancelRun(
	ctx context.Context,
	req *connect.Request[threadv1.CancelRunRequest],
) (*connect.Response[threadv1.CancelRunResponse], error) {
	ctx, r, err := s.reachableRun(ctx, req.Msg.GetRunId())
	if err != nil {
		return nil, err
	}
	if err := s.runs.Cancel(ctx, r.ID); err != nil {
		return nil, err
	}
	if r, err = s.runs.Run(ctx, r.ID); err != nil {
		return nil, err
	}
	out, err := s.runWithThread(ctx, r)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&threadv1.CancelRunResponse{Run: out}), nil
}

// SECURITY: unknown and out-of-reach threads answer the same not-found; a reached thread scopes the call to its org and project.
func (s *ThreadService) reachableThread(ctx context.Context, publicID string) (context.Context, Ref, error) {
	ref, err := s.runs.ResolveThread(ctx, publicID)
	if IsThreadNotFoundError(err) || (err == nil && !authz.ReachResource(ctx, ref.OrgID, ref.ProjectID, ref.AgentID)) {
		return ctx, Ref{}, &ThreadNotFoundError{}
	}
	if err != nil {
		return ctx, Ref{}, err
	}
	return tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID}), ref, nil
}

// SECURITY: unknown and out-of-reach runs answer the same not-found; a reached run scopes the call to its org and project.
func (s *ThreadService) reachableRun(ctx context.Context, publicID string) (context.Context, *Run, error) {
	ref, err := s.runs.ResolveRun(ctx, publicID)
	if IsNotFoundError(err) || (err == nil && !authz.ReachResource(ctx, ref.OrgID, ref.ProjectID, ref.AgentID)) {
		return ctx, nil, &NotFoundError{}
	}
	if err != nil {
		return ctx, nil, err
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID})
	r, err := s.runs.Run(ctx, ref.ID)
	return ctx, r, err
}

func (s *ThreadService) threadProto(ctx context.Context, sum *ThreadSummary, projectPub string) (*threadv1.Thread, error) {
	agentPub, _, err := s.refs.ResourcePublicID(ctx, sum.OrgID, sum.AgentID)
	if err != nil {
		return nil, err
	}
	out := &threadv1.Thread{
		Id: sum.PublicID, ProjectId: projectPub, AgentId: agentPub, Kind: sum.Kind,
		OpenRunStatus: statusToProto(sum.OpenRun), MessageCount: sum.Messages, RunCount: sum.Runs,
	}
	if !sum.LastActivityAt.IsZero() {
		out.LastActivityAt = timestamppb.New(sum.LastActivityAt)
	}
	if sum.ActorEndUserID != uuid.Nil {
		p, err := s.people.Person(ctx, sum.ActorEndUserID)
		if err != nil {
			return nil, err
		}
		out.EndUserLabel, out.EndUserDisplayName, out.EndUserIdentityKind, out.EndUserIdentityHint = p.Label, p.DisplayName, p.Kind, p.Hint
	}
	return out, nil
}

func (s *ThreadService) runWithThread(ctx context.Context, r *Run) (*threadv1.Run, error) {
	th, err := s.runs.store.Thread(ctx, r.ThreadID)
	if err != nil {
		return nil, s.runs.errOrReport(ctx, "run: thread of run", err)
	}
	return s.runProto(ctx, r, th.PublicID)
}

func (s *ThreadService) runProto(ctx context.Context, r *Run, threadPub string) (*threadv1.Run, error) {
	version, draft, err := s.runs.AgentVersion(ctx, r.AgentVersionID)
	if err != nil {
		return nil, err
	}
	kind := versionPublished
	if draft {
		kind = versionDraft
	}
	out := &threadv1.Run{
		Id: r.PublicID, ThreadId: threadPub, Status: statusToProto(r.Status), Attempt: r.Attempt, AgentVersion: version, AgentVersionKind: kind,
		Usage: &threadv1.Usage{
			InputTokens: r.Usage.InputTokens, OutputTokens: r.Usage.OutputTokens,
			CacheReadTokens: r.Usage.CacheReadTokens, CacheWriteTokens: r.Usage.CacheWriteTokens, Estimated: r.Usage.Estimated,
			EmbeddingTokens:   r.Usage.EmbeddingTokens,
			HelperInputTokens: r.Usage.HelperInputTokens, HelperOutputTokens: r.Usage.HelperOutputTokens,
			HelperCacheReadTokens: r.Usage.HelperCacheReadTokens, HelperCacheWriteTokens: r.Usage.HelperCacheWriteTokens,
		},
		ErrorCode: r.ErrorCode, NoReplyReason: r.NoReplyReason, StartedAt: timestamppb.New(r.StartedAt),
		KnowledgeRetrieveCalls: r.KnowledgeRetrieveCalls, KnowledgeRetrieveMs: r.KnowledgeRetrieveMs,
		KnowledgePassages: r.KnowledgePassages,
	}
	if cost, ok := Cost(r.Usage, r.Prices); ok {
		out.CostMicros = &cost
	}
	if r.EndedAt != nil {
		out.EndedAt = timestamppb.New(*r.EndedAt)
		out.LatencyMs = new(r.EndedAt.Sub(r.StartedAt).Milliseconds())
	}
	return out, nil
}

func (s *ThreadService) runPublicID(ctx context.Context, seen map[uuid.UUID]string, id uuid.UUID) (string, error) {
	if pub, ok := seen[id]; ok {
		return pub, nil
	}
	r, err := s.runs.Run(ctx, id)
	if err != nil {
		return "", err
	}
	seen[id] = r.PublicID
	return r.PublicID, nil
}

func (s *ThreadService) person(ctx context.Context, seen map[uuid.UUID]Person, id uuid.UUID) (Person, error) {
	if p, ok := seen[id]; ok {
		return p, nil
	}
	p, err := s.people.Person(ctx, id)
	if err != nil {
		return Person{}, err
	}
	seen[id] = p
	return p, nil
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
	if err != nil {
		return Cursor{}, &InvalidPageTokenError{}
	}
	at, pub, ok := strings.Cut(string(raw), "|")
	if !ok || pub == "" {
		return Cursor{}, &InvalidPageTokenError{}
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return Cursor{}, &InvalidPageTokenError{}
	}
	return Cursor{At: t, PublicID: pub}, nil
}

func bodyProto(body json.RawMessage) *structpb.Struct {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return nil
	}
	out, err := structpb.NewStruct(m)
	if err != nil {
		return nil
	}
	return out
}

func statusToProto(st Status) threadv1.RunStatus {
	switch st {
	case StatusRunning:
		return threadv1.RunStatus_RUN_STATUS_RUNNING
	case StatusAwaitingApproval:
		return threadv1.RunStatus_RUN_STATUS_AWAITING_APPROVAL
	case StatusCompleted:
		return threadv1.RunStatus_RUN_STATUS_COMPLETED
	case StatusFailed:
		return threadv1.RunStatus_RUN_STATUS_FAILED
	case StatusCancelled:
		return threadv1.RunStatus_RUN_STATUS_CANCELLED
	case StatusInterrupted:
		return threadv1.RunStatus_RUN_STATUS_INTERRUPTED
	case StatusLimitReached:
		return threadv1.RunStatus_RUN_STATUS_LIMIT_REACHED
	case StatusExpired:
		return threadv1.RunStatus_RUN_STATUS_EXPIRED
	default:
		return threadv1.RunStatus_RUN_STATUS_UNSPECIFIED
	}
}
