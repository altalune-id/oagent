package fakes

import (
	"context"
	"slices"
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
		ID: threadID, PublicID: f.PublicIDs[threadID], OrgID: th.OrgID, ProjectID: th.ProjectID, AgentID: f.Agents[threadID], Kind: "api",
		CancelRequested: th.CancelRequested,
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

// Claim implements run.Store.
func (f *RunStore) Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if run.IsThreadNotFoundError(err) || f.ClaimRefused {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if th.LeaseOwner != "" && th.LeaseUntil.After(f.now()) {
		return false, nil
	}
	th.LeaseOwner, th.LeaseUntil = token, f.now().Add(ttl)
	return true, nil
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
func (f *RunStore) RequestCancel(ctx context.Context, threadID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	th, err := f.thread(ctx, threadID)
	if err != nil {
		return err
	}
	th.CancelRequested = true
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
