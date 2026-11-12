package dataplane

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scope"
)

type threadLookup func(ctx context.Context, agentID uuid.UUID, externalKey string) (*run.Thread, error)

// SECURITY: a thread is the caller's only when its bound actor is the caller's end user; any other thread reads as not found.
func (h *Handler) ownThread(c caller, in turnInput, actor string, lookup threadLookup) (*run.Thread, error) {
	thread, err := lookup(c.ctx, c.agent.ID, in.ThreadID)
	if err != nil {
		return nil, err
	}
	endUser, found, err := h.d.EndUsers.FindAPIUser(c.ctx, c.agent.ProjectID, actor)
	if err != nil {
		return nil, err
	}
	if !found || thread.ActorEndUserID != endUser {
		return nil, &run.ThreadActorMismatchError{ThreadID: thread.ID.String()}
	}
	return thread, nil
}

// NOTE: refusePending answers a POST with no resume on the caller's thread that waits on an approval with APR001, before any write.
func (h *Handler) refusePending(c caller, in turnInput, actor string, lookup threadLookup) error {
	if h.d.Approvals == nil {
		return nil
	}
	thread, err := h.ownThread(c, in, actor, lookup)
	if _, mismatch := errors.AsType[*run.ThreadActorMismatchError](err); mismatch || run.IsThreadNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pending, err := h.d.Approvals.PendingForThread(c.ctx, thread.ID)
	if err != nil || pending == nil {
		return err
	}
	return &approval.PendingError{Approval: pending.PublicID}
}

// SECURITY: needs approval:decide and the bound actor; Reserve, lease, Decide with no kick, RunHeld: the POST streams its own run.
func (h *Handler) resume(c caller, in turnInput, actor string, lookup threadLookup) (*run.Sink, error) {
	if h.d.Approvals == nil {
		return nil, invalid("resume", apperror.FieldNotAllowed)
	}
	if !scope.Grants(c.principal.Scopes, scope.ApprovalDecide) {
		return nil, forbidden(scope.ApprovalDecide)
	}
	thread, err := h.ownThread(c, in, actor, lookup)
	if err != nil {
		return nil, err
	}
	pending, err := h.d.Approvals.PendingForThread(c.ctx, thread.ID)
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return nil, &approval.NotFoundError{}
	}
	decision, err := resumeAnswer(in.Resume, pending.Open)
	if err != nil {
		return nil, err
	}
	if !h.d.RuntimeReady() {
		return nil, retryLater(http.StatusServiceUnavailable, errNoRuntime)
	}
	slot, ok := h.d.Runner.Reserve()
	if !ok {
		return nil, retryLater(http.StatusServiceUnavailable, errRunnerFull)
	}
	held := false
	defer func() {
		if !held {
			slot.Release()
		}
	}()
	token := uuid.NewString()
	claim, err := h.d.Store.Claim(c.ctx, thread.ID, token, h.d.LeaseTTL)
	switch {
	case err != nil:
		return nil, err
	case claim == run.LeaseLive:
		return nil, &run.InProgressError{}
	case claim == run.ProjectBusy:
		return nil, retryLater(http.StatusTooManyRequests, errProjectBusy)
	}
	release := func() { _ = h.d.Store.Release(context.WithoutCancel(c.ctx), thread.ID, token) }
	if err := h.d.Approvals.Decide(c.ctx, pending.ID, decision.Approved, decision.Text); err != nil {
		release()
		return nil, err
	}
	sink := run.NewSink(run.SinkCapacity)
	held = true
	if err := h.d.Runner.RunHeld(slot, run.Kick{OrgID: c.agent.OrgID, ThreadID: thread.ID}, token, sink); err != nil {
		release()
		return nil, retryLater(http.StatusServiceUnavailable, errRunnerFull)
	}
	return sink, nil
}
