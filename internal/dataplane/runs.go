package dataplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scope"
)

// IdempotencyHeader carries the client's key for a POST it may retry.
const IdempotencyHeader = "Idempotency-Key"

const retryAfter = 2 * time.Second

var (
	errAgentNotFound = apperror.New(apperror.CodeAgentNotFound, "Agent not found")
	errRunnerFull    = apperror.New(apperror.CodeRunRuntimeUnavailable, "Every turn slot is busy; try again shortly")
	errNoRuntime     = apperror.New(apperror.CodeRunRuntimeUnavailable, "The agent runtime is not configured")
	errProjectBusy   = apperror.New(apperror.CodeRunRateLimited, "The project is running as many turns as it may; try again shortly")
	errNotPublished  = apperror.New(apperror.CodeAgentNeedsRepublish, "The agent has no published version; publish it before running it")
)

func forbidden(permission string) error {
	return apperror.New(apperror.CodeForbidden, "Missing permission "+permission).
		WithMeta(map[string]string{"permission": permission})
}

type caller struct {
	ctx       context.Context
	principal session.Principal
	agent     AgentRef
}

func (h *Handler) authorize(r *http.Request, permission string) (caller, error) {
	if err := h.d.Limits.ip(r); err != nil {
		return caller{}, err
	}
	cred, present := authn.FromHeader(r.Header, h.d.KeyPrefix, nil)
	if !present || cred.Kind != authn.KindAPIKey {
		return caller{}, errNoKey
	}
	p, err := h.d.Keys.Authenticate(r.Context(), cred)
	if err != nil {
		return caller{}, err
	}
	if err := h.d.Limits.key(r.Context(), p.KeyID.String()); err != nil {
		return caller{}, err
	}
	return h.reachAgent(session.PrincipalInto(r.Context(), p), r, p, permission)
}

func (h *Handler) reachAgent(ctx context.Context, r *http.Request, p session.Principal, permission string) (caller, error) {
	ref, found, err := h.d.Agents.Resolve(ctx, r.PathValue("agent"))
	if err != nil {
		return caller{}, err
	}
	// SECURITY: an agent of another org or project, or one the key does not reach, is not found, never forbidden.
	if !found || ref.OrgID != p.ActiveOrgID || !authz.ReachResource(ctx, ref.OrgID, ref.ProjectID, ref.ID) {
		return caller{}, errAgentNotFound
	}
	if !scope.Grants(p.Scopes, permission) {
		return caller{}, forbidden(permission)
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID})
	return caller{ctx: ctx, principal: p, agent: ref}, nil
}

func (h *Handler) postRuns(w http.ResponseWriter, r *http.Request) {
	c, err := h.authorize(r, scope.RunCreate)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := decodeInput(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.keepReading(w, r)
	actor, err := h.actorValue(c.ctx, r, c.principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if in.Resume != nil {
		stream, err := h.resume(c, in, actor, h.d.Store.ThreadByExternalKey)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		h.relay(w, r, stream, in.ThreadID)
		return
	}
	if err := h.refusePending(c, in, actor, h.d.Store.ThreadByExternalKey); err != nil {
		h.writeError(w, r, err)
		return
	}
	stream, err := h.start(c, r, in, actor)
	if replay, ok := errors.AsType[*settledReplay](err); ok {
		if err := h.replay(c.ctx, w, in.ThreadID, replay.run.ThreadID, replay.run); err != nil {
			h.d.Log.WarnContext(r.Context(), "dataplane: replay", "err", err.Error())
		}
		return
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.relay(w, r, stream, in.ThreadID)
}

// NOTE: net/http clears the read deadline once the body hits EOF; this keeps a stream past ReadTimeout even if that changes.
func (h *Handler) keepReading(w http.ResponseWriter, r *http.Request) {
	if err := http.NewResponseController(w).SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.d.Log.DebugContext(r.Context(), "dataplane: clear read deadline", "err", err.Error())
	}
}

func (h *Handler) relay(w http.ResponseWriter, r *http.Request, stream *run.Sink, threadID string) {
	streamHeaders(w)
	if err := Relay(r.Context(), stream.Events(), NewSSEWriter(w), IDMap{ThreadID: threadID}, nil); err != nil {
		h.d.Log.DebugContext(r.Context(), "dataplane: stream ended early; the run goes on", "err", err.Error())
	}
}

// NOTE: every refusal that needs no stored state comes before the first write, so a refused POST leaves no rows.
func (h *Handler) start(c caller, r *http.Request, in turnInput, actor string) (*run.Sink, error) {
	if c.agent.Draft {
		return nil, errNotPublished
	}
	key := r.Header.Get(IdempotencyHeader)
	if key != "" && !idPattern.MatchString(key) {
		return nil, invalid(IdempotencyHeader, apperror.FieldInvalidFormat)
	}
	if key != "" {
		if err := h.knownBeforeWriting(c, in, actor, key); err != nil {
			return nil, err
		}
	}
	return h.open(c, in, actor, key, h.d.Store.UpsertAPIThread)
}

type upsertThread func(ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID) (*run.Thread, error)

func (h *Handler) open(c caller, in turnInput, actor, key string, upsert upsertThread) (*run.Sink, error) {
	if !h.d.RuntimeReady() {
		return nil, retryLater(http.StatusServiceUnavailable, errNoRuntime)
	}
	if err := h.d.Rate.Allow(c.ctx, c.agent.ProjectID); err != nil {
		if limited, ok := errors.AsType[*run.RateLimitedError](err); ok {
			return nil, tooMany(limited.RetryAfter, err)
		}
		return nil, withStatus(http.StatusServiceUnavailable, err)
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
	endUser, err := h.d.EndUsers.UpsertAPIUser(c.ctx, c.agent.ProjectID, actor)
	if err != nil {
		return nil, err
	}
	thread, err := upsert(c.ctx, c.agent.ProjectID, c.agent.ID, in.ThreadID, endUser)
	if err != nil {
		return nil, err
	}
	msg, err := userMessage(in, endUser)
	if err != nil {
		return nil, err
	}
	if key != "" {
		msg.ExternalMessageID, msg.RequestHash = run.IdempotencyPrefix+key, requestHash(in)
	}
	token := uuid.NewString()
	claim, _, err := h.d.Store.ClaimAndAppend(c.ctx, thread.ID, token, h.d.LeaseTTL, msg)
	switch {
	case err != nil:
		if dup, ok := errors.AsType[*run.DuplicateMessageError](err); ok {
			return nil, h.duplicate(c.ctx, thread.ID, msg, dup)
		}
		return nil, err
	case claim == run.LeaseLive:
		return nil, &run.InProgressError{}
	case claim == run.ProjectBusy:
		return nil, retryLater(http.StatusTooManyRequests, errProjectBusy)
	}
	sink := run.NewSink(run.SinkCapacity)
	held = true
	if err := h.d.Runner.RunHeld(slot, run.Kick{OrgID: c.agent.OrgID, ThreadID: thread.ID}, token, sink); err != nil {
		_ = h.d.Store.Release(context.WithoutCancel(c.ctx), thread.ID, token)
		return nil, retryLater(http.StatusServiceUnavailable, errRunnerFull)
	}
	return sink, nil
}

// SECURITY: a read-only look for a retried request; a thread bound to another actor, or to none, reads as not found.
func (h *Handler) knownBeforeWriting(c caller, in turnInput, actor, key string) error {
	thread, err := h.d.Store.ThreadByExternalKey(c.ctx, c.agent.ID, in.ThreadID)
	if run.IsThreadNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	endUser, found, err := h.d.EndUsers.FindAPIUser(c.ctx, c.agent.ProjectID, actor)
	if err != nil {
		return err
	}
	if !found || thread.ActorEndUserID != endUser {
		return &run.ThreadActorMismatchError{ThreadID: thread.ID.String()}
	}
	return h.knownRequest(c.ctx, thread.ID, run.Message{ExternalMessageID: run.IdempotencyPrefix + key, RequestHash: requestHash(in)})
}

func userMessage(in turnInput, speaker uuid.UUID) (run.Message, error) {
	id := in.MessageID
	if id == "" {
		id = uuid.NewString()
	}
	body, err := json.Marshal(map[string]string{"id": id, "role": "user", "content": in.Text})
	if err != nil {
		return run.Message{}, err
	}
	return run.Message{Role: run.RoleUser, Body: body, SpeakerID: &speaker}, nil
}

func (h *Handler) knownRequest(ctx context.Context, threadID uuid.UUID, msg run.Message) error {
	known, err := h.d.Store.MessageByExternalID(ctx, threadID, msg.ExternalMessageID)
	if err != nil || known == nil {
		return err
	}
	return h.duplicate(ctx, threadID, msg, &run.DuplicateMessageError{Seq: known.Seq})
}

func (h *Handler) duplicate(ctx context.Context, threadID uuid.UUID, msg run.Message, _ *run.DuplicateMessageError) error {
	known, err := h.d.Store.MessageByExternalID(ctx, threadID, msg.ExternalMessageID)
	if err != nil {
		return err
	}
	if known != nil && !bytes.Equal(known.RequestHash, msg.RequestHash) {
		return &run.IdempotencyConflictError{}
	}
	key := strings.TrimPrefix(msg.ExternalMessageID, run.IdempotencyPrefix)
	r, err := h.d.Store.RunByIdempotencyKey(ctx, threadID, key)
	if err != nil {
		return err
	}
	if r == nil {
		return &run.InProgressError{}
	}
	if r.Status.Terminal() {
		return &settledReplay{run: r}
	}
	return &run.InProgressError{RunPublicID: r.PublicID}
}
