package fakes

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/run"
)

// Cancelled is one approval a run's cancel rejected, with whether its unit of work was open.
type Cancelled struct {
	ID             uuid.UUID
	InTx, BySystem bool
}

// Raised is one approval a run raised, with whether its unit of work was open.
type Raised struct {
	In   run.RaiseApproval
	InTx bool
}

// RunApprovals records raised approvals and answers Latest from Rows, newest last; Err fails every call.
type RunApprovals struct {
	Err     error
	mu      sync.Mutex
	raised  []Raised
	answers []run.ChatAnswer
	cancels []Cancelled
	Rows    map[uuid.UUID][]run.RunApproval
	// LatestErr fails Latest only.
	LatestErr error
	// BeforeChat runs at the start of DecideFromChat, to decide the row elsewhere first.
	BeforeChat func()
}

// Raise implements run.Approvals and keeps the approval pending as the run's latest.
func (f *RunApprovals) Raise(ctx context.Context, in run.RaiseApproval) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.raised = append(f.raised, Raised{In: in, InTx: InTx(ctx)})
	if f.Rows == nil {
		f.Rows = map[uuid.UUID][]run.RunApproval{}
	}
	channel := "api"
	if in.ThreadKind == "dm" || in.ThreadKind == "group" {
		channel = "whatsapp"
	}
	f.Rows[in.RunID] = append(f.Rows[in.RunID], run.RunApproval{
		ID: uuid.New(), PublicID: "apr_" + uuid.NewString()[:17], Preview: "the preview of " + in.Calls[0].Tool, Status: "pending",
		Calls: in.Calls, RaisedSnapshotHash: in.SnapshotHash, ExpiresAt: time.Now().Add(time.Hour),
		Channel: channel, Requester: approval.RequesterOf(in.Speakers),
	})
	return nil
}

// MatchWord implements run.Approvals with the approval word list.
func (f *RunApprovals) MatchWord(text string) (approved, matched bool) {
	w := approval.MatchWord(text)
	return w == approval.WordYes, w != approval.WordNone
}

// Expire implements run.Approvals: a pending row past its expiry becomes expired.
func (f *RunApprovals) Expire(_ context.Context, id uuid.UUID) (bool, error) {
	if f.Err != nil {
		return false, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rows := range f.Rows {
		for i := range rows {
			if rows[i].ID == id && rows[i].Status == "pending" && !time.Now().Before(rows[i].ExpiresAt) {
				rows[i].Status = "expired"
				return true, nil
			}
		}
	}
	return false, nil
}

// DecideFromChat implements run.Approvals: it decides the pending row with a.ApprovalID and records the answer.
func (f *RunApprovals) DecideFromChat(_ context.Context, a run.ChatAnswer) (bool, error) {
	if f.BeforeChat != nil {
		f.BeforeChat()
	}
	if f.Err != nil {
		return false, f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for runID, rows := range f.Rows {
		for i := range rows {
			if rows[i].ID != a.ApprovalID {
				continue
			}
			if rows[i].Status != "pending" {
				return false, nil
			}
			rows[i].Status, rows[i].DecisionText = "rejected", a.Text
			if a.Approved {
				rows[i].Status = "approved"
				for j := range rows[i].Calls {
					rows[i].Calls[j].State = "approved"
				}
			}
			f.Rows[runID] = rows
			f.answers = append(f.answers, a)
			return true, nil
		}
	}
	return false, nil
}

// Answers returns every chat decision recorded.
func (f *RunApprovals) Answers() []run.ChatAnswer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]run.ChatAnswer(nil), f.answers...)
}

// PromptSent sets the run's latest row's sent prompt id and watermark, as the replier would.
func (f *RunApprovals) PromptSent(runID uuid.UUID, externalID string, afterSeq int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.Rows[runID]
	rows[len(rows)-1].PromptExternalID, rows[len(rows)-1].PromptAfterSeq = externalID, &afterSeq
}

// Latest implements run.Approvals: the run's newest row, or nil.
func (f *RunApprovals) Latest(_ context.Context, runID uuid.UUID) (*run.RunApproval, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.LatestErr != nil {
		return nil, f.LatestErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.Rows[runID]
	if len(rows) == 0 {
		return nil, nil
	}
	out := rows[len(rows)-1]
	out.Calls = append([]run.ApprovalCall(nil), out.Calls...)
	return &out, nil
}

// Decide sets the run's latest row to status with text, as a person's decision would.
func (f *RunApprovals) Decide(runID uuid.UUID, status, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.Rows[runID]
	rows[len(rows)-1].Status, rows[len(rows)-1].DecisionText = status, text
	if status == "approved" {
		for i := range rows[len(rows)-1].Calls {
			rows[len(rows)-1].Calls[i].State = "approved"
		}
	}
}

// CallState is the state of call toolCallID on the run's latest row.
func (f *RunApprovals) CallState(runID uuid.UUID, toolCallID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.Rows[runID]
	for _, c := range rows[len(rows)-1].Calls {
		if c.ToolCallID == toolCallID {
			return c.State
		}
	}
	return ""
}

// RaisedSoFar returns what was raised.
func (f *RunApprovals) RaisedSoFar() []Raised {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Raised(nil), f.raised...)
}

// Cancelled returns every approval a cancel rejected.
func (f *RunApprovals) Cancelled() []Cancelled {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Cancelled(nil), f.cancels...)
}

// ExpireUnresumable implements run.Approvals: a pending or approved row expires with its run.
func (f *RunApprovals) ExpireUnresumable(_ context.Context, id uuid.UUID) error {
	return f.set(id, []string{"pending", "approved"}, "expired")
}

// RejectOnCancel implements run.Approvals: a pending row is rejected, and the call is recorded.
func (f *RunApprovals) RejectOnCancel(ctx context.Context, id uuid.UUID, bySystem bool) error {
	if err := f.set(id, []string{"pending"}, "rejected"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, Cancelled{ID: id, InTx: InTx(ctx), BySystem: bySystem})
	return nil
}

func (f *RunApprovals) set(id uuid.UUID, from []string, to string) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rows := range f.Rows {
		for i := range rows {
			if rows[i].ID == id && slices.Contains(from, rows[i].Status) {
				rows[i].Status = to
			}
		}
	}
	return nil
}

// MarkExecuting implements run.Approvals.
func (f *RunApprovals) MarkExecuting(_ context.Context, id uuid.UUID, toolCallIDs []string) error {
	return f.move(id, toolCallIDs, "approved", "executing")
}

// MarkExecuted implements run.Approvals.
func (f *RunApprovals) MarkExecuted(_ context.Context, id uuid.UUID, toolCallID string) error {
	return f.move(id, []string{toolCallID}, "executing", "executed")
}

// MarkUnknown implements run.Approvals.
func (f *RunApprovals) MarkUnknown(_ context.Context, id uuid.UUID, toolCallID string) error {
	return f.move(id, []string{toolCallID}, "executing", "unknown")
}

func (f *RunApprovals) move(id uuid.UUID, toolCallIDs []string, from, to string) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rows := range f.Rows {
		for i := range rows {
			if rows[i].ID != id {
				continue
			}
			for _, want := range toolCallIDs {
				j := slices.IndexFunc(rows[i].Calls, func(c run.ApprovalCall) bool { return c.ToolCallID == want })
				if j < 0 || rows[i].Calls[j].State != from {
					return fmt.Errorf("fakes: call %s is not %s", want, from)
				}
				rows[i].Calls[j].State = to
			}
			return nil
		}
	}
	return fmt.Errorf("fakes: no approval %s", id)
}

// NeedsApproval implements run.Approvals with the approval evaluator, like boot's adapter.
func (f *RunApprovals) NeedsApproval(grant run.ToolGrant, args json.RawMessage) bool {
	return approval.NeedsApproval(grant.RequireApproval, grant.FreeWhen, args)
}

// MarkUndelivered implements run.Approvals.
func (f *RunApprovals) MarkUndelivered(_ context.Context, id uuid.UUID, toolCallIDs []string) error {
	return f.move(id, toolCallIDs, "executing", "approved")
}
