package boot

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/user"
)

// NOTE: approvalRefs adds the thread and run public ids, read from the run store, to the shared public id lookups.
type approvalRefs struct {
	publicIDs
	svcs *Services
}

func (r approvalRefs) ThreadPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	th, err := r.svcs.RunStore.Thread(ctx, id)
	if err != nil {
		return "", err
	}
	return th.PublicID, nil
}

func (r approvalRefs) RunPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	got, err := r.svcs.RunStore.RunByID(ctx, id)
	if err != nil {
		return "", err
	}
	return got.PublicID, nil
}

// NOTE: approvalNames labels an approval's people with the end-user label rule; a deleted member or key has an empty label.
type approvalNames struct{ svcs *Services }

func (n approvalNames) EndUser(ctx context.Context, id uuid.UUID) (publicID, label string, err error) {
	e, err := n.svcs.EndUsers.Get(ctx, id)
	if err != nil {
		return "", "", err
	}
	name, err := n.svcs.EndUsers.Name(ctx, id)
	if err != nil {
		return "", "", err
	}
	return e.PublicID, name.Label(), nil
}

func (n approvalNames) Member(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := n.svcs.Users.ByID(ctx, userID)
	if user.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return u.Name, nil
}

func (n approvalNames) KeyHint(ctx context.Context, keyID uuid.UUID) (string, error) {
	return keyHints{keys: n.svcs.APIKeyStore}.KeyHint(ctx, keyID)
}

// NOTE: approvalKicker wakes the thread after a console, MCP or WhatsApp decision, through the queue when it is on.
type approvalKicker struct{ svcs *Services }

func (k approvalKicker) Kick(ctx context.Context, orgID, threadID uuid.UUID) error {
	if k.svcs.RunKicker == nil {
		return nil
	}
	return k.svcs.RunKicker.Kick(ctx, run.Kick{OrgID: orgID, ThreadID: threadID})
}

// NOTE: runApprovals lets the run raise an approval inside its settle and read the latest one on resume, in run's own types.
type runApprovals struct{ svcs *Services }

func (a runApprovals) Raise(ctx context.Context, in run.RaiseApproval) error {
	calls := make([]approval.Call, len(in.Calls))
	for i, c := range in.Calls {
		calls[i] = approval.Call{
			ToolCallID: c.ToolCallID, InterruptID: c.InterruptID, Server: c.Server, Tool: c.Tool, Args: c.Args, ArgsHash: c.ArgsHash, Keys: c.Keys,
		}
	}
	_, err := a.svcs.Approvals.Create(ctx, approval.CreateInput{
		OrgID: in.OrgID, ProjectID: in.ProjectID, AgentID: in.AgentID, RunID: in.RunID, ThreadID: in.ThreadID, ThreadKind: in.ThreadKind,
		Calls: calls, Speakers: in.Speakers, RaisedSnapshotHash: in.SnapshotHash,
	})
	return err
}

func (a runApprovals) Latest(ctx context.Context, runID uuid.UUID) (*run.RunApproval, error) {
	got, err := a.svcs.Approvals.LatestForRun(ctx, runID)
	if approval.IsNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	calls := make([]run.ApprovalCall, len(got.Calls))
	for i, c := range got.Calls {
		calls[i] = run.ApprovalCall{
			ToolCallID: c.ToolCallID, InterruptID: c.InterruptID, Server: c.Server, Tool: c.Tool, Args: c.Args, ArgsHash: c.ArgsHash, Keys: c.Keys,
			State: string(c.State),
		}
	}
	return &run.RunApproval{
		ID: got.ID, PublicID: got.PublicID, Preview: got.Preview, Status: string(got.Status), DecisionText: got.DecisionText,
		ExpiresAt: got.ExpiresAt, Calls: calls,
		RaisedSnapshotHash: got.RaisedSnapshotHash, Channel: string(got.Channel), Requester: got.Requester,
		PromptExternalID: got.PromptExternalID, PromptAfterSeq: got.PromptAfterSeq,
	}, nil
}

func (runApprovals) MatchWord(text string) (approved, matched bool) {
	w := approval.MatchWord(text)
	return w == approval.WordYes, w != approval.WordNone
}

func (a runApprovals) Expire(ctx context.Context, id uuid.UUID) (bool, error) {
	return a.svcs.Approvals.Expire(ctx, id)
}

// SECURITY: the audit fails closed: arguments or conditions that don't decode count as needing approval.
func (runApprovals) NeedsApproval(grant run.ToolGrant, args json.RawMessage) bool {
	return approval.NeedsApproval(grant.RequireApproval, grant.FreeWhen, args)
}

func (a runApprovals) MarkExecuting(ctx context.Context, id uuid.UUID, toolCallIDs []string) error {
	return a.svcs.Approvals.MarkExecuting(ctx, id, toolCallIDs)
}

func (a runApprovals) MarkUndelivered(ctx context.Context, id uuid.UUID, toolCallIDs []string) error {
	return a.svcs.Approvals.MarkUndelivered(ctx, id, toolCallIDs)
}

func (a runApprovals) MarkExecuted(ctx context.Context, id uuid.UUID, toolCallID string) error {
	return a.svcs.Approvals.MarkExecuted(ctx, id, toolCallID)
}

func (a runApprovals) MarkUnknown(ctx context.Context, id uuid.UUID, toolCallID string) error {
	return a.svcs.Approvals.MarkUnknown(ctx, id, toolCallID)
}

func (a runApprovals) ExpireUnresumable(ctx context.Context, id uuid.UUID) error {
	return a.svcs.Approvals.ExpireUnresumable(ctx, id)
}

// NOTE: a cancel rejects as the canceller; past its expiry the row is left to the approval-expire job.
func (a runApprovals) RejectOnCancel(ctx context.Context, id uuid.UUID, bySystem bool) error {
	by := approval.DeciderOf(ctx)
	if bySystem {
		by = approval.Decider{Kind: approval.DeciderSystem}
	}
	_, err := a.svcs.Approvals.Decide(ctx, id, approval.Answer{By: by})
	if approval.IsAlreadyDecidedError(err) {
		return nil
	}
	return err
}

func (a runApprovals) DecideFromChat(ctx context.Context, in run.ChatAnswer) (bool, error) {
	_, err := a.svcs.Approvals.Decide(ctx, in.ApprovalID, approval.Answer{
		Approved: in.Approved, Text: in.Text, By: approval.Decider{Kind: approval.DeciderEndUser, ID: in.By.String()}, Via: approval.ViaWhatsApp,
	})
	if approval.IsAlreadyDecidedError(err) {
		return false, nil
	}
	return err == nil, err
}

// NOTE: approvalPrompts gives the channel replier an awaiting run's pending approval and records where its prompt went.
type approvalPrompts struct{ svcs *Services }

func (a approvalPrompts) Prompt(ctx context.Context, runID uuid.UUID) (*channel.Prompt, error) {
	got, err := a.svcs.Approvals.LatestForRun(ctx, runID)
	if approval.IsNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if got.Status != approval.StatusPending || !got.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	return &channel.Prompt{ApprovalID: got.ID, PublicID: got.PublicID, Preview: approval.WhatsAppText(got.Preview)}, nil
}

func (a approvalPrompts) PromptSent(ctx context.Context, approvalID uuid.UUID, externalID string, afterSeq int64) error {
	return a.svcs.Approvals.SetPromptSent(ctx, approvalID, externalID, afterSeq)
}
