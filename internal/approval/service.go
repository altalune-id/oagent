package approval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/nanoid"
)

// Service creates, decides and reads approvals.
type Service struct {
	store      Store
	uow        db.Transactor
	refs       Refs
	unexpected apperror.UnexpectedFunc
	publisher  Publisher
	kicker     Kicker
	now        func() time.Time
}

// NewService builds the approval service; refs names public ids for events.
func NewService(store Store, uow db.Transactor, refs Refs, unexpected apperror.UnexpectedFunc) *Service {
	if refs == nil {
		panic("approval: NewService needs Refs")
	}
	return &Service{store: store, uow: uow, refs: refs, unexpected: unexpected, now: time.Now}
}

// WithPublisher emits approval.requested and approval.decided through p.
func (s *Service) WithPublisher(p Publisher) *Service {
	s.publisher = p
	return s
}

// WithKicker wakes a thread's runner after a decision that asks for it.
func (s *Service) WithKicker(k Kicker) *Service {
	s.kicker = k
	return s
}

// WithClock replaces the clock, for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// CreateInput is a run's gated calls at the awaiting settle; Speakers are the turn's taken messages' speakers in order.
type CreateInput struct {
	OrgID, ProjectID, AgentID, RunID, ThreadID uuid.UUID
	ThreadKind                                 string
	Calls                                      []Call
	Speakers                                   []uuid.NullUUID
	RaisedSnapshotHash                         []byte
}

// Decider is who decided an approval; ID is the user's, API key's or end user's id, empty for the system.
type Decider struct {
	Kind DeciderKind
	ID   string
}

// Answer is a person's decision; Kick wakes the runner after it commits, and the API path, which holds the lease, leaves it off.
type Answer struct {
	Approved bool
	Text     string
	By       Decider
	Via      Via
	Kick     bool
}

// Create records a pending approval for the calls and emits approval.requested in the caller's unit of work.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Approval, error) {
	if len(in.Calls) == 0 || len(in.Calls) > maxCalls {
		return nil, s.errOrReport(ctx, "approval: create", fmt.Errorf("approval: %d calls, want 1 to %d", len(in.Calls), maxCalls))
	}
	publicID, err := nanoid.New(21)
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: create", err)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	a := &Approval{
		ID: uuid.New(), PublicID: publicID, OrgID: in.OrgID, ProjectID: in.ProjectID, AgentID: in.AgentID, RunID: in.RunID,
		ThreadID: in.ThreadID, Calls: in.Calls, Preview: Preview(in.Calls), Channel: ChannelOf(in.ThreadKind),
		Requester: RequesterOf(in.Speakers), RaisedSnapshotHash: in.RaisedSnapshotHash, Status: StatusPending,
		ExpiresAt: now.Add(ExpiryOf(in.ThreadKind)), CreatedAt: now,
	}
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.Insert(ctx, a); err != nil {
			return err
		}
		return s.publish(ctx, events.ApprovalRequested, a)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: create", err)
	}
	return a, nil
}

// Decide answers a pending, unexpired approval once and emits approval.decided in the same unit of work.
func (s *Service) Decide(ctx context.Context, id uuid.UUID, d Answer) (*Approval, error) {
	now := s.now().UTC().Truncate(time.Microsecond)
	status := StatusRejected
	if d.Approved {
		status = StatusApproved
	}
	var out *Approval
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		// SECURITY: single use: the store writes only while pending and unexpired, so a second or late decision matches 0 rows.
		a, ok, err := s.store.Decide(ctx, id, Decided{Status: status, By: d.By, Via: d.Via, Text: capRunes(d.Text, maxDecisionText), At: now})
		if err != nil {
			return err
		}
		if !ok {
			return s.alreadyDecided(ctx, id, now)
		}
		out = a
		return s.publish(ctx, events.ApprovalDecided, a)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: decide", err)
	}
	if d.Kick && s.kicker != nil {
		if err := s.kicker.Kick(ctx, out.OrgID, out.ThreadID); err != nil {
			// NOTE: the decision is committed; the sweep resumes the thread if this kick is lost.
			_ = s.errOrReport(ctx, "approval: kick", err)
		}
	}
	return out, nil
}

func (s *Service) alreadyDecided(ctx context.Context, id uuid.UUID, now time.Time) error {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	status := cur.Status
	if status == StatusPending && !now.Before(cur.ExpiresAt) {
		status = StatusExpired
	}
	return &AlreadyDecidedError{Status: string(status)}
}

// Get loads an approval of the bound org.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Approval, error) {
	a, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: get", err)
	}
	return a, nil
}

// List pages a project's approvals newest first.
func (s *Service) List(ctx context.Context, f Filter, after Cursor, limit int) ([]*Approval, error) {
	f.Now = s.now().UTC()
	out, err := s.store.List(ctx, f, after, limit)
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: list", err)
	}
	return out, nil
}

// CountPending counts a project's pending approvals of the given agents, or of every agent when agents is empty.
func (s *Service) CountPending(ctx context.Context, projectID uuid.UUID, agents []uuid.UUID) (int, error) {
	n, err := s.store.CountPending(ctx, projectID, agents, s.now().UTC())
	if err != nil {
		return 0, s.errOrReport(ctx, "approval: count pending", err)
	}
	return n, nil
}

// Resolve names publicID's approval and where it lives, in any org; the caller checks reach before it reads it.
func (s *Service) Resolve(ctx context.Context, publicID string) (Ref, error) {
	r, err := s.store.Resolve(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "approval: resolve", err)
	}
	return r, nil
}

// LatestForRun is the run's newest approval, or a *NotFoundError when it raised none.
func (s *Service) LatestForRun(ctx context.Context, runID uuid.UUID) (*Approval, error) {
	a, err := s.store.LatestForRun(ctx, runID)
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: latest", err)
	}
	return a, nil
}

// Expire marks id expired when it is pending and past its expiry on the database clock; it emits approval.decided in the same unit of work.
func (s *Service) Expire(ctx context.Context, id uuid.UUID) (bool, error) {
	a, err := s.expire(ctx, id)
	return a != nil, err
}

// ExpireUnresumable expires id with a run that can't resume; a pending or approved row becomes expired and emits approval.decided.
func (s *Service) ExpireUnresumable(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		a, ok, err := s.store.ExpireUnresumable(ctx, id)
		if err != nil || !ok {
			return err
		}
		return s.publish(ctx, events.ApprovalDecided, a)
	})
	if err != nil {
		return s.errOrReport(ctx, "approval: expire unresumable", err)
	}
	return nil
}

// NOTE: expire is the one expiry path: the run's resume and the approval-expire job both take it.
func (s *Service) expire(ctx context.Context, id uuid.UUID) (*Approval, error) {
	var out *Approval
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		a, ok, err := s.store.Expire(ctx, id)
		if err != nil || !ok {
			return err
		}
		out = a
		return s.publish(ctx, events.ApprovalDecided, a)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: expire", err)
	}
	return out, nil
}

// SetPromptSent records approval id's sent prompt: its provider id, for quoted group replies, and the seq after which messages may decide.
func (s *Service) SetPromptSent(ctx context.Context, id uuid.UUID, externalID string, afterSeq int64) error {
	if err := s.store.SetPromptSent(ctx, id, externalID, afterSeq); err != nil {
		return s.errOrReport(ctx, "approval: prompt id", err)
	}
	return nil
}

// PendingForThread is the thread's pending approval, or a *NotFoundError when there is none.
func (s *Service) PendingForThread(ctx context.Context, threadID uuid.UUID) (*Approval, error) {
	a, err := s.store.PendingForThread(ctx, threadID)
	if err != nil {
		return nil, s.errOrReport(ctx, "approval: pending", err)
	}
	return a, nil
}

// MarkExecuting moves approved calls to executing before the resume is sent; the caller commits it under the lease.
func (s *Service) MarkExecuting(ctx context.Context, id uuid.UUID, toolCallIDs []string) error {
	return s.errOrReport(ctx, "approval: executing", s.store.SetCallStates(ctx, id, toolCallIDs, CallApproved, CallExecuting))
}

// MarkUndelivered moves executing calls back to approved when their request provably never reached the runtime.
func (s *Service) MarkUndelivered(ctx context.Context, id uuid.UUID, toolCallIDs []string) error {
	return s.errOrReport(ctx, "approval: undelivered", s.store.SetCallStates(ctx, id, toolCallIDs, CallExecuting, CallApproved))
}

// MarkExecuted records that an executing call's result was stored.
func (s *Service) MarkExecuted(ctx context.Context, id uuid.UUID, toolCallID string) error {
	return s.errOrReport(ctx, "approval: executed", s.store.SetCallStates(ctx, id, []string{toolCallID}, CallExecuting, CallExecuted))
}

// MarkUnknown records that an executing call has no stored result after a restart, so it never runs again.
func (s *Service) MarkUnknown(ctx context.Context, id uuid.UUID, toolCallID string) error {
	return s.errOrReport(ctx, "approval: unknown", s.store.SetCallStates(ctx, id, []string{toolCallID}, CallExecuting, CallUnknown))
}

// NOTE: a try run sends no webhook, so its approvals emit no event either.
func (s *Service) publish(ctx context.Context, t events.Type, a *Approval) error {
	if s.publisher == nil || a.Channel == ChannelConsole {
		return nil
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: a.OrgID, ProjectID: a.ProjectID})
	payload := events.ApprovalV1{ID: a.PublicID, Tools: a.Tools(), Status: string(a.Status), Channel: string(a.Channel)}
	var ok bool
	var err error
	if payload.ProjectID, err = s.refs.ProjectPublicID(ctx, a.ProjectID); err != nil {
		return fmt.Errorf("approval: publish %s: %w", t, err)
	}
	if payload.AgentID, ok, err = s.refs.ResourcePublicID(ctx, a.OrgID, a.AgentID); err != nil || !ok {
		return fmt.Errorf("approval: publish %s: agent: %w", t, errors.Join(err, errMissingRef))
	}
	if payload.ThreadID, err = s.refs.ThreadPublicID(ctx, a.ThreadID); err != nil {
		return fmt.Errorf("approval: publish %s: %w", t, err)
	}
	if payload.RunID, err = s.refs.RunPublicID(ctx, a.RunID); err != nil {
		return fmt.Errorf("approval: publish %s: %w", t, err)
	}
	var data any = events.ApprovalRequestedV1(payload)
	if t == events.ApprovalDecided {
		data = events.ApprovalDecidedV1(payload)
	}
	if err := s.publisher.Enqueue(ctx, t, data); err != nil {
		return fmt.Errorf("approval: publish %s: %w", t, err)
	}
	return nil
}

var errMissingRef = errors.New("no public id")

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsNotFoundError(err) || IsAlreadyDecidedError(err) || s.unexpected == nil {
		return err
	}
	return s.unexpected(ctx, message, err)
}
