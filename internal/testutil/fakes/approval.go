package fakes

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/approval"
)

// ApprovalStore keeps approvals in memory with the Postgres store's single-use rule; Err fails every call.
type ApprovalStore struct {
	Err  error
	mu   sync.Mutex
	rows map[uuid.UUID]*approval.Approval
}

func (f *ApprovalStore) clone(a *approval.Approval) *approval.Approval {
	c := *a
	c.Calls = slices.Clone(a.Calls)
	return &c
}

// Insert implements approval.Store; a second pending approval on a thread is refused, like the partial unique index.
func (f *ApprovalStore) Insert(_ context.Context, a *approval.Approval) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows == nil {
		f.rows = map[uuid.UUID]*approval.Approval{}
	}
	for _, r := range f.rows {
		if r.ThreadID == a.ThreadID && r.Status == approval.StatusPending {
			return fmt.Errorf("fakes: thread %s already has a pending approval", a.ThreadID)
		}
	}
	f.rows[a.ID] = f.clone(a)
	return nil
}

// Get implements approval.Store.
func (f *ApprovalStore) Get(_ context.Context, id uuid.UUID) (*approval.Approval, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		return f.clone(r), nil
	}
	return nil, &approval.NotFoundError{}
}

// List implements approval.Store, newest first.
func (f *ApprovalStore) List(_ context.Context, flt approval.Filter, after approval.Cursor, limit int) ([]*approval.Approval, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*approval.Approval
	for _, r := range f.rows {
		switch {
		case r.ProjectID != flt.ProjectID,
			len(flt.AgentIDs) > 0 && !slices.Contains(flt.AgentIDs, r.AgentID),
			flt.Status == approval.StatusDecided && r.Status != approval.StatusApproved && r.Status != approval.StatusRejected,
			flt.Status != "" && flt.Status != approval.StatusDecided && r.Status != flt.Status,
			flt.Status == approval.StatusPending && !flt.Now.Before(r.ExpiresAt),
			!after.At.IsZero() && !r.CreatedAt.Before(after.At) && (!r.CreatedAt.Equal(after.At) || r.PublicID >= after.PublicID):
			continue
		}
		out = append(out, f.clone(r))
	}
	slices.SortFunc(out, func(a, b *approval.Approval) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.PublicID, a.PublicID)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// CountPending implements approval.Store.
func (f *ApprovalStore) CountPending(_ context.Context, projectID uuid.UUID, agents []uuid.UUID, now time.Time) (int, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.rows {
		live := r.Status == approval.StatusPending && now.Before(r.ExpiresAt)
		if r.ProjectID == projectID && live && (len(agents) == 0 || slices.Contains(agents, r.AgentID)) {
			n++
		}
	}
	return n, nil
}

// Resolve implements approval.Store, across every org.
func (f *ApprovalStore) Resolve(_ context.Context, publicID string) (approval.Ref, error) {
	if f.Err != nil {
		return approval.Ref{}, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.PublicID == publicID {
			return approval.Ref{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID}, nil
		}
	}
	return approval.Ref{}, &approval.NotFoundError{}
}

// LatestForRun implements approval.Store, newest by creation.
func (f *ApprovalStore) LatestForRun(_ context.Context, runID uuid.UUID) (*approval.Approval, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out *approval.Approval
	for _, r := range f.rows {
		if r.RunID == runID && (out == nil || r.CreatedAt.After(out.CreatedAt)) {
			out = r
		}
	}
	if out == nil {
		return nil, &approval.NotFoundError{}
	}
	return f.clone(out), nil
}

// PendingForThread implements approval.Store.
func (f *ApprovalStore) PendingForThread(_ context.Context, threadID uuid.UUID) (*approval.Approval, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ThreadID == threadID && r.Status == approval.StatusPending {
			return f.clone(r), nil
		}
	}
	return nil, &approval.NotFoundError{}
}

// Decide implements approval.Store: it writes only while the row is pending and unexpired at d.At.
func (f *ApprovalStore) Decide(_ context.Context, id uuid.UUID, d approval.Decided) (*approval.Approval, bool, error) {
	if f.Err != nil {
		return nil, false, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.Status != approval.StatusPending || !d.At.Before(r.ExpiresAt) {
		return nil, false, nil
	}
	r.Status, r.DecidedByKind, r.DecidedByID, r.DecidedVia, r.DecisionText = d.Status, d.By.Kind, d.By.ID, d.Via, d.Text
	at := d.At
	r.DecidedAt = &at
	if d.Status == approval.StatusApproved {
		for i := range r.Calls {
			r.Calls[i].State = approval.CallApproved
		}
	}
	return f.clone(r), true, nil
}

// SetCallStates implements approval.Store.
func (f *ApprovalStore) SetCallStates(_ context.Context, id uuid.UUID, toolCallIDs []string, from, to approval.CallState) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return &approval.NotFoundError{}
	}
	for _, want := range toolCallIDs {
		i := slices.IndexFunc(r.Calls, func(c approval.Call) bool { return c.ToolCallID == want })
		if i < 0 || r.Calls[i].State != from {
			return fmt.Errorf("fakes: call %s is not %s", want, from)
		}
	}
	for _, want := range toolCallIDs {
		i := slices.IndexFunc(r.Calls, func(c approval.Call) bool { return c.ToolCallID == want })
		r.Calls[i].State = to
	}
	return nil
}

// SetPromptSent records a non-empty externalID and afterSeq where the row has none.
func (f *ApprovalStore) SetPromptSent(_ context.Context, id uuid.UUID, externalID string, afterSeq int64) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return &approval.NotFoundError{}
	}
	if r.PromptExternalID == "" {
		r.PromptExternalID = externalID
	}
	if r.PromptAfterSeq == nil {
		r.PromptAfterSeq = &afterSeq
	}
	return nil
}

// Expire marks a pending row past its expiry expired by the system.
func (f *ApprovalStore) Expire(_ context.Context, id uuid.UUID) (*approval.Approval, bool, error) {
	if f.Err != nil {
		return nil, false, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	now := time.Now()
	if !ok || r.Status != approval.StatusPending || now.Before(r.ExpiresAt) {
		return nil, false, nil
	}
	r.Status, r.DecidedByKind, r.DecidedAt = approval.StatusExpired, approval.DeciderSystem, &now
	out := *r
	return &out, true, nil
}

// DueExpiries lists pending rows past their expiry, oldest first.
func (f *ApprovalStore) DueExpiries(_ context.Context, limit int) ([]uuid.UUID, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var due []*approval.Approval
	now := time.Now()
	for _, r := range f.rows {
		if r.Status == approval.StatusPending && !now.Before(r.ExpiresAt) {
			due = append(due, r)
		}
	}
	slices.SortFunc(due, func(a, b *approval.Approval) int { return a.ExpiresAt.Compare(b.ExpiresAt) })
	out := make([]uuid.UUID, 0, len(due))
	for _, r := range due[:min(len(due), max(limit, 1))] {
		out = append(out, r.ID)
	}
	return out, nil
}

// ExpireUnresumable marks a pending or approved row expired, keeping a decider and time already set.
func (f *ApprovalStore) ExpireUnresumable(_ context.Context, id uuid.UUID) (*approval.Approval, bool, error) {
	if f.Err != nil {
		return nil, false, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || (r.Status != approval.StatusPending && r.Status != approval.StatusApproved) {
		return nil, false, nil
	}
	if r.DecidedByKind == "" {
		r.DecidedByKind = approval.DeciderSystem
	}
	if r.DecidedAt == nil {
		now := time.Now()
		r.DecidedAt = &now
	}
	r.Status = approval.StatusExpired
	out := *r
	return &out, true, nil
}
