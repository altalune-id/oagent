package fakes

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
)

// RunThread is a thread as the fake run store keeps it.
type RunThread struct {
	OrgID, ProjectID uuid.UUID
	LeaseOwner       string
	LeaseUntil       time.Time
	CancelRequested  bool
	NextSeq          int64
}

// RunStore is an in-memory run.Store, safe for concurrent use; Err fails every call and Now defaults to time.Now.
type RunStore struct {
	Err      error
	Now      func() time.Time
	mu       sync.Mutex
	Threads  map[uuid.UUID]*RunThread
	Runs     map[uuid.UUID]*run.Run
	Messages []run.Message
	// Agents holds each thread's agent, for Thread.
	Agents map[uuid.UUID]uuid.UUID
	// PublicIDs holds each thread's public id, for Thread.
	PublicIDs map[uuid.UUID]string
	// CheckpointErr and SettleErr fail only those calls.
	CheckpointErr, SettleErr error
	// ResumeGone makes a resume find its run no longer interrupted.
	ResumeGone bool
	// ClaimRefused makes every Claim lose, as a project at its concurrency cap does.
	ClaimRefused bool
	// ExternalKeys and Actors hold each API thread's key and bound end user.
	ExternalKeys map[uuid.UUID]string
	Actors       map[uuid.UUID]uuid.UUID
	// Connections holds each channel thread's connection.
	Connections map[uuid.UUID]uuid.UUID
	// Texts holds each connection's set replies; TextsErr fails ReplyTexts.
	Texts    map[uuid.UUID]run.ReplyTexts
	TextsErr error
	// Kinds maps a thread to its kind; a thread not in it is an API thread.
	Kinds map[uuid.UUID]string
	// Activity holds each thread's last activity, for ListThreads; a missing thread reads as the zero time.
	Activity map[uuid.UUID]time.Time
}

var _ run.Store = (*RunStore)(nil)

// AddThread registers a thread of orgID and projectID.
func (f *RunStore) AddThread(id, orgID, projectID uuid.UUID) {
	f.AddAgentThread(id, orgID, projectID, uuid.Nil)
}

// AddAgentThread registers a thread of agentID in orgID and projectID.
func (f *RunStore) AddAgentThread(id, orgID, projectID, agentID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Threads == nil {
		f.Threads = map[uuid.UUID]*RunThread{}
	}
	f.Threads[id] = &RunThread{OrgID: orgID, ProjectID: projectID, NextSeq: 1}
	if f.Agents == nil {
		f.Agents = map[uuid.UUID]uuid.UUID{}
	}
	f.Agents[id] = agentID
}

// Thread implements run.Store.
func (f *RunStore) Thread(ctx context.Context, threadID uuid.UUID) (*run.Thread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return &run.Thread{
		ID: threadID, PublicID: f.PublicIDs[threadID], OrgID: th.OrgID, ProjectID: th.ProjectID, AgentID: f.Agents[threadID],
		Kind:            f.kind(threadID),
		CancelRequested: th.CancelRequested, ExternalKey: f.ExternalKeys[threadID], ActorEndUserID: f.Actors[threadID],
		ConnectionID: f.Connections[threadID],
	}, nil
}

// History implements run.Store.
func (f *RunStore) History(ctx context.Context, threadID, exceptRun uuid.UUID) ([]run.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.thread(ctx, threadID); err != nil {
		return nil, err
	}
	var out []run.Message
	for _, m := range f.Messages {
		if m.ThreadID == threadID && m.Status == run.MessageConsumed && (m.RunID == nil || *m.RunID != exceptRun) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *RunStore) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *RunStore) thread(ctx context.Context, id uuid.UUID) (*RunThread, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	th, ok := f.Threads[id]
	if !ok || th.OrgID != tc.OrgID {
		return nil, &run.ThreadNotFoundError{ID: id.String()}
	}
	return th, nil
}

func (f *RunStore) fenced(ctx context.Context, id uuid.UUID, token string) (*RunThread, error) {
	th, err := f.thread(ctx, id)
	if run.IsThreadNotFoundError(err) || (err == nil && th.LeaseOwner != token) {
		return nil, &run.LeaseLostError{ThreadID: id.String()}
	}
	return th, err
}

func (f *RunStore) append(id uuid.UUID, th *RunThread, msgs []run.Message) {
	for _, m := range msgs {
		if m.ID == uuid.Nil {
			m.ID = uuid.New()
		}
		m.OrgID, m.ProjectID, m.ThreadID, m.Seq, m.CreatedAt = th.OrgID, th.ProjectID, id, th.NextSeq, f.now()
		th.NextSeq++
		f.Messages = append(f.Messages, m)
	}
}

// AppendMessage implements run.Store.
func (f *RunStore) AppendMessage(ctx context.Context, threadID uuid.UUID, m run.Message) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if err != nil {
		return 0, err
	}
	for _, known := range f.Messages {
		if m.ExternalMessageID != "" && known.ThreadID == threadID && known.ExternalMessageID == m.ExternalMessageID {
			return known.Seq, nil
		}
	}
	m.Status, m.RunID = run.MessagePending, nil
	f.append(threadID, th, []run.Message{m})
	return th.NextSeq - 1, nil
}

// Claim implements run.Store; ClaimRefused answers ProjectBusy.
func (f *RunStore) Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (run.ClaimResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claim(ctx, threadID, token, ttl)
}

func (f *RunStore) claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (run.ClaimResult, error) {
	th, err := f.thread(ctx, threadID)
	if run.IsThreadNotFoundError(err) {
		return run.LeaseLive, nil
	}
	if err != nil {
		return run.LeaseLive, err
	}
	if th.LeaseOwner != "" && th.LeaseUntil.After(f.now()) {
		return run.LeaseLive, nil
	}
	if f.ClaimRefused {
		return run.ProjectBusy, nil
	}
	th.LeaseOwner, th.LeaseUntil = token, f.now().Add(ttl)
	return run.Claimed, nil
}

// ClaimAndAppend implements run.Store.
func (f *RunStore) ClaimAndAppend(
	ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration, m run.Message,
) (run.ClaimResult, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m.ExternalMessageID != "" {
		for _, known := range f.Messages {
			if known.ThreadID == threadID && known.ExternalMessageID == m.ExternalMessageID {
				return run.LeaseLive, 0, &run.DuplicateMessageError{Seq: known.Seq}
			}
		}
	}
	result, err := f.claim(ctx, threadID, token, ttl)
	if err != nil || result != run.Claimed {
		return result, 0, err
	}
	th := f.Threads[threadID]
	m.Status, m.RunID = run.MessagePending, nil
	f.append(threadID, th, []run.Message{m})
	return run.Claimed, th.NextSeq - 1, nil
}

// UpsertAPIThread implements run.Store.
func (f *RunStore) UpsertAPIThread(
	ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID,
) (*run.Thread, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	for id, key := range f.ExternalKeys {
		if key == externalKey && f.Agents[id] == agentID && f.Threads[id].OrgID == tc.OrgID {
			f.mu.Unlock()
			if actor == uuid.Nil || f.Actors[id] != actor {
				return nil, &run.ThreadActorMismatchError{ThreadID: id.String()}
			}
			return f.Thread(ctx, id)
		}
	}
	f.mu.Unlock()
	id := uuid.New()
	f.AddAgentThread(id, tc.OrgID, projectID, agentID)
	f.mu.Lock()
	if f.ExternalKeys == nil {
		f.ExternalKeys, f.Actors = map[uuid.UUID]string{}, map[uuid.UUID]uuid.UUID{}
	}
	f.ExternalKeys[id], f.Actors[id] = externalKey, actor
	f.mu.Unlock()
	return f.Thread(ctx, id)
}

// ThreadByExternalKey implements run.Store.
func (f *RunStore) ThreadByExternalKey(ctx context.Context, agentID uuid.UUID, externalKey string) (*run.Thread, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	var found uuid.UUID
	for id, key := range f.ExternalKeys {
		if key == externalKey && f.Agents[id] == agentID && f.Threads[id].OrgID == tc.OrgID {
			found = id
		}
	}
	f.mu.Unlock()
	if found == uuid.Nil {
		return nil, &run.ThreadNotFoundError{ID: externalKey}
	}
	return f.Thread(ctx, found)
}

// RunByPublicID implements run.Store.
func (f *RunStore) RunByPublicID(ctx context.Context, publicID string) (*run.Run, error) {
	return f.findRun(ctx, func(r *run.Run) bool { return r.PublicID == publicID }, &run.NotFoundError{ID: publicID})
}

// RunByIdempotencyKey implements run.Store.
func (f *RunStore) RunByIdempotencyKey(ctx context.Context, threadID uuid.UUID, key string) (*run.Run, error) {
	r, err := f.findRun(ctx, func(r *run.Run) bool { return r.ThreadID == threadID && r.IdempotencyKey == key }, nil)
	if run.IsNotFoundError(err) {
		return nil, nil
	}
	return r, err
}

func (f *RunStore) findRun(ctx context.Context, match func(*run.Run) bool, missing error) (*run.Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.Runs {
		if r.OrgID == tc.OrgID && match(r) {
			out := *r
			return &out, nil
		}
	}
	if missing == nil {
		missing = &run.NotFoundError{}
	}
	return nil, missing
}

// MessageByExternalID implements run.Store.
func (f *RunStore) MessageByExternalID(ctx context.Context, threadID uuid.UUID, externalID string) (*run.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.thread(ctx, threadID); err != nil {
		return nil, err
	}
	for _, m := range f.Messages {
		if m.ThreadID == threadID && m.ExternalMessageID == externalID {
			out := m
			return &out, nil
		}
	}
	return nil, nil
}

// Heartbeat implements run.Store.
func (f *RunStore) Heartbeat(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.fenced(ctx, threadID, token)
	if err != nil {
		return false, err
	}
	th.LeaseUntil = f.now().Add(ttl)
	return th.CancelRequested, nil
}

// Release implements run.Store.
func (f *RunStore) Release(ctx context.Context, threadID uuid.UUID, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.fenced(ctx, threadID, token)
	if err != nil {
		return err
	}
	th.LeaseOwner, th.LeaseUntil = "", time.Time{}
	return nil
}

// RequestCancel implements run.Store.
func (f *RunStore) RequestCancel(ctx context.Context, threadID, runID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if err != nil {
		return err
	}
	if r, ok := f.Runs[runID]; ok && r.ThreadID == threadID && !r.Status.Terminal() {
		th.CancelRequested = true
	}
	return nil
}

// Reclaim implements run.Store.
func (f *RunStore) Reclaim(ctx context.Context, threadID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if run.IsThreadNotFoundError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if th.LeaseOwner != "" && !th.LeaseUntil.Before(f.now()) {
		return false, nil
	}
	reclaimed := th.LeaseOwner != ""
	th.LeaseOwner, th.LeaseUntil = "", time.Time{}
	for _, r := range f.Runs {
		if r.ThreadID == threadID && r.Status == run.StatusRunning {
			r.Status, r.UpdatedAt = run.StatusInterrupted, f.now()
			reclaimed = true
		}
	}
	return reclaimed, nil
}

// OpenRun implements run.Store.
func (f *RunStore) OpenRun(ctx context.Context, threadID uuid.UUID) (*run.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.thread(ctx, threadID); err != nil {
		if run.IsThreadNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, r := range f.Runs {
		if r.ThreadID == threadID && r.Status == run.StatusInterrupted {
			out := *r
			return &out, nil
		}
	}
	return nil, nil
}

// StartRun implements run.Store.
func (f *RunStore) StartRun(ctx context.Context, token string, r *run.Run) ([]run.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.fenced(ctx, r.ThreadID, token)
	if err != nil {
		return nil, err
	}
	if r.Attempt > 1 && f.ResumeGone {
		return nil, run.ErrNothingPending
	}
	if r.Attempt == 1 && !slices.ContainsFunc(f.Messages, func(m run.Message) bool {
		return m.ThreadID == r.ThreadID && m.Status == run.MessagePending
	}) {
		return nil, run.ErrNothingPending
	}
	if r.Attempt == 1 {
		th.CancelRequested = false
	}
	if f.Runs == nil {
		f.Runs = map[uuid.UUID]*run.Run{}
	}
	saved := *r
	f.Runs[r.ID] = &saved
	var taken []run.Message
	for i, m := range f.Messages {
		resumed := r.Attempt > 1 && m.RunID != nil && *m.RunID == r.ID && m.ToolCallID == ""
		fresh := r.Attempt == 1 && m.ThreadID == r.ThreadID && m.Status == run.MessagePending
		if fresh {
			f.Messages[i].Status, f.Messages[i].RunID = run.MessageConsumed, &r.ID
		}
		if fresh || resumed {
			taken = append(taken, f.Messages[i])
		}
	}
	if key := run.IdempotencyKeyOf(taken); key != "" && r.Attempt == 1 && r.IdempotencyKey == "" {
		r.IdempotencyKey = key
		f.Runs[r.ID].IdempotencyKey = key
	}
	return taken, nil
}

// PutCheckpoint implements run.Store.
func (f *RunStore) PutCheckpoint(ctx context.Context, token string, runID uuid.UUID, toolCallID string, msgs []run.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if f.CheckpointErr != nil {
		return f.CheckpointErr
	}
	r, ok := f.Runs[runID]
	if !ok {
		return &run.NotFoundError{ID: runID.String()}
	}
	th, err := f.fenced(ctx, r.ThreadID, token)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(f.Messages, func(m run.Message) bool {
		return m.RunID != nil && *m.RunID == runID && m.ToolCallID == toolCallID
	}) {
		return nil
	}
	f.append(r.ThreadID, th, consumed(runID, msgs))
	return nil
}

func consumed(runID uuid.UUID, msgs []run.Message) []run.Message {
	out := slices.Clone(msgs)
	for i := range out {
		out[i].Status, out[i].RunID = run.MessageConsumed, &runID
	}
	return out
}

// StoredResults implements run.Store.
func (f *RunStore) StoredResults(_ context.Context, runID uuid.UUID) ([]run.StoredResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	var out []run.StoredResult
	for _, m := range f.Messages {
		if m.RunID != nil && *m.RunID == runID && m.ToolCallID != "" {
			out = append(out, run.StoredResult{ToolUseID: m.ToolCallID, Body: m.Body})
		}
	}
	return out, nil
}

// Settle implements run.Store.
func (f *RunStore) Settle(ctx context.Context, token string, r *run.Run, u run.Usage, final []run.Message) error {
	if !r.Status.Terminal() && r.Status != run.StatusInterrupted {
		return &run.UnsettledError{Status: r.Status}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SettleErr != nil {
		return f.SettleErr
	}
	th, err := f.fenced(ctx, r.ThreadID, token)
	if err != nil {
		return err
	}
	f.append(r.ThreadID, th, consumed(r.ID, final))
	r.Usage = u
	saved := *r
	f.Runs[r.ID] = &saved
	th.CancelRequested = false
	return nil
}

// Expired implements run.Store.
func (f *RunStore) Expired(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	return f.threads(ctx, func(id uuid.UUID, th *RunThread) bool {
		if th.LeaseOwner != "" {
			return th.LeaseUntil.Before(now)
		}
		for _, r := range f.Runs {
			if r.ThreadID == id && r.Status == run.StatusRunning {
				return true
			}
		}
		return false
	}, 0)
}

// PendingUnleased implements run.Store.
func (f *RunStore) PendingUnleased(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	return f.threads(ctx, func(id uuid.UUID, th *RunThread) bool {
		unleased := th.LeaseOwner == "" || th.LeaseUntil.Before(now)
		pending := slices.ContainsFunc(f.Messages, func(m run.Message) bool {
			return m.ThreadID == id && m.Status == run.MessagePending
		})
		resumable := false
		for _, r := range f.Runs {
			resumable = resumable || (r.ThreadID == id && r.Status == run.StatusInterrupted)
		}
		return unleased && (pending || resumable)
	}, limit)
}

func (f *RunStore) threads(ctx context.Context, keep func(uuid.UUID, *RunThread) bool, limit int) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for id, th := range f.Threads {
		if th.OrgID == tc.OrgID && keep(id, th) {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// AllRuns returns a copy of every stored run.
func (f *RunStore) AllRuns() []run.Run {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]run.Run, 0, len(f.Runs))
	for _, r := range f.Runs {
		out = append(out, *r)
	}
	return out
}

// LeaseOwner returns the token holding the thread's lease, empty when none does.
func (f *RunStore) LeaseOwner(threadID uuid.UUID) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if th, ok := f.Threads[threadID]; ok {
		return th.LeaseOwner
	}
	return ""
}

// HasPending implements run.Store.
func (f *RunStore) HasPending(ctx context.Context, threadID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.thread(ctx, threadID); err != nil {
		return false, err
	}
	return slices.ContainsFunc(f.Messages, func(m run.Message) bool {
		return m.ThreadID == threadID && m.Status == run.MessagePending
	}), nil
}

// RunByID implements run.Store.
func (f *RunStore) RunByID(ctx context.Context, id uuid.UUID) (*run.Run, error) {
	return f.findRun(ctx, func(r *run.Run) bool { return r.ID == id }, &run.NotFoundError{ID: id.String()})
}

// ThreadMessages implements run.Store.
func (f *RunStore) ThreadMessages(ctx context.Context, threadID uuid.UUID, afterSeq int64, limit int) ([]run.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.thread(ctx, threadID); err != nil {
		if run.IsThreadNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []run.Message
	for _, m := range f.Messages {
		if m.ThreadID == threadID && m.Seq > afterSeq && len(out) < max(limit, 1) {
			out = append(out, m)
		}
	}
	return out, nil
}

// ResolveThread implements run.Store; like the Postgres resolver it needs no tenant.
func (f *RunStore) ResolveThread(_ context.Context, publicID string) (run.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return run.Ref{}, f.Err
	}
	for id, pub := range f.PublicIDs {
		if th, ok := f.Threads[id]; ok && pub == publicID {
			return run.Ref{ID: id, OrgID: th.OrgID, ProjectID: th.ProjectID, AgentID: f.Agents[id]}, nil
		}
	}
	return run.Ref{}, &run.ThreadNotFoundError{ID: publicID}
}

// ResolveRun implements run.Store; like the Postgres resolver it needs no tenant.
func (f *RunStore) ResolveRun(_ context.Context, publicID string) (run.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return run.Ref{}, f.Err
	}
	for _, r := range f.Runs {
		if r.PublicID == publicID {
			return run.Ref{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: f.Agents[r.ThreadID]}, nil
		}
	}
	return run.Ref{}, &run.NotFoundError{ID: publicID}
}

func (f *RunStore) summary(id uuid.UUID, th *RunThread) run.ThreadSummary {
	out := run.ThreadSummary{
		Thread: run.Thread{
			ID: id, PublicID: f.PublicIDs[id], OrgID: th.OrgID, ProjectID: th.ProjectID, AgentID: f.Agents[id], Kind: f.kind(id),
			ExternalKey: f.ExternalKeys[id], ActorEndUserID: f.Actors[id], CancelRequested: th.CancelRequested,
		},
		LastActivityAt: f.Activity[id],
	}
	for _, r := range f.Runs {
		if r.ThreadID == id {
			out.Runs++
			if !r.Status.Terminal() {
				out.OpenRun = r.Status
			}
		}
	}
	for _, m := range f.Messages {
		if m.ThreadID == id {
			out.Messages++
		}
	}
	return out
}

func newerFirst(aAt, bAt time.Time, aID, bID string) int {
	if c := bAt.Compare(aAt); c != 0 {
		return c
	}
	return strings.Compare(bID, aID)
}

func before(c run.Cursor, at time.Time, id string) bool {
	return c.At.IsZero() || newerFirst(at, c.At, id, c.PublicID) > 0
}

// ListThreads implements run.Store.
func (f *RunStore) ListThreads(
	ctx context.Context, projectID, agentID uuid.UUID, after run.Cursor, limit int,
) ([]run.ThreadSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []run.ThreadSummary
	for id, th := range f.Threads {
		if th.OrgID != tc.OrgID || th.ProjectID != projectID || (agentID != uuid.Nil && f.Agents[id] != agentID) {
			continue
		}
		sum := f.summary(id, th)
		if before(after, sum.LastActivityAt, sum.PublicID) {
			sum.Messages, sum.Runs = 0, 0
			out = append(out, sum)
		}
	}
	slices.SortFunc(out, func(a, b run.ThreadSummary) int {
		return newerFirst(a.LastActivityAt, b.LastActivityAt, a.PublicID, b.PublicID)
	})
	return out[:min(len(out), max(limit, 1))], nil
}

// ThreadSummary implements run.Store.
func (f *RunStore) ThreadSummary(ctx context.Context, threadID uuid.UUID) (*run.ThreadSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	sum := f.summary(threadID, th)
	return &sum, nil
}

// ListRuns implements run.Store.
func (f *RunStore) ListRuns(ctx context.Context, threadID uuid.UUID, after run.Cursor, limit int) ([]run.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []run.Run
	for _, r := range f.Runs {
		if r.ThreadID == threadID && r.OrgID == tc.OrgID && before(after, r.StartedAt, r.PublicID) {
			out = append(out, *r)
		}
	}
	slices.SortFunc(out, func(a, b run.Run) int { return newerFirst(a.StartedAt, b.StartedAt, a.PublicID, b.PublicID) })
	return out[:min(len(out), max(limit, 1))], nil
}

// AppendChannelMessage implements run.Store; it is not modeled in memory.
func (f *RunStore) AppendChannelMessage(context.Context, run.ChannelMessage) (run.ChannelAppend, error) {
	return run.ChannelAppend{}, errors.New("fakes: AppendChannelMessage is not modeled")
}

func (f *RunStore) kind(id uuid.UUID) string {
	if k, ok := f.Kinds[id]; ok {
		return k
	}
	return "api"
}

// ReplyTexts implements run.Store.
func (f *RunStore) ReplyTexts(_ context.Context, connectionID uuid.UUID) (run.ReplyTexts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.TextsErr != nil {
		return run.ReplyTexts{}, f.TextsErr
	}
	return f.Texts[connectionID], nil
}

// ListAgentRuns implements run.Store.
func (f *RunStore) ListAgentRuns(ctx context.Context, agentID uuid.UUID, after run.Cursor, limit int) ([]run.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []run.Run
	for _, r := range f.Runs {
		if f.Agents[r.ThreadID] == agentID && r.OrgID == tc.OrgID && before(after, r.StartedAt, r.PublicID) {
			out = append(out, *r)
		}
	}
	slices.SortFunc(out, func(a, b run.Run) int { return newerFirst(a.StartedAt, b.StartedAt, a.PublicID, b.PublicID) })
	return out[:min(len(out), max(limit, 1))], nil
}

// EndUserActivity implements run.Store over the stored messages and thread actors.
func (f *RunStore) EndUserActivity(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]run.EndUserActivity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uuid.UUID]run.EndUserActivity{}
	for _, id := range ids {
		threads := map[uuid.UUID]bool{}
		var a run.EndUserActivity
		for _, m := range f.Messages {
			if m.SpeakerID != nil && *m.SpeakerID == id {
				threads[m.ThreadID] = true
				if m.CreatedAt.After(a.LastSeen) {
					a.LastSeen = m.CreatedAt
				}
			}
		}
		for thread, actor := range f.Actors {
			if actor == id {
				threads[thread] = true
			}
		}
		a.Threads = int64(len(threads))
		out[id] = a
	}
	return out, nil
}

// EndUserThreads implements run.Store over the stored messages and thread actors.
func (f *RunStore) EndUserThreads(ctx context.Context, id uuid.UUID) ([]run.ThreadSummary, error) {
	seen, err := f.EndUserActivity(ctx, []uuid.UUID{id})
	if err != nil || seen[id].Threads == 0 {
		return nil, err
	}
	f.mu.Lock()
	ids := map[uuid.UUID]bool{}
	for _, m := range f.Messages {
		if m.SpeakerID != nil && *m.SpeakerID == id {
			ids[m.ThreadID] = true
		}
	}
	for thread, actor := range f.Actors {
		if actor == id {
			ids[thread] = true
		}
	}
	f.mu.Unlock()
	var out []run.ThreadSummary
	for thread := range ids {
		th, err := f.Thread(ctx, thread)
		if err != nil {
			return nil, err
		}
		out = append(out, run.ThreadSummary{Thread: *th})
	}
	return out, nil
}
