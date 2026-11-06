package dataplane

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scope"
)

const (
	defaultPage = 50
	maxPage     = 200
)

var (
	errRunNotFound    = (&run.NotFoundError{}).ToAppError()
	errThreadNotFound = (&run.ThreadNotFoundError{}).ToAppError()
)

type usageView struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	Estimated        bool  `json:"estimated"`
}

type runView struct {
	ID         string     `json:"id"`
	ThreadID   string     `json:"thread_id"`
	Status     string     `json:"status"`
	Attempt    int32      `json:"attempt"`
	Usage      usageView  `json:"usage"`
	CostMicros *int64     `json:"cost_micros"`
	ErrorCode  string     `json:"error_code,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at"`
}

func viewOf(r *run.Run, threadKey string) runView {
	v := runView{
		ID: r.PublicID, ThreadID: threadKey, Status: string(r.Status), Attempt: r.Attempt, ErrorCode: r.ErrorCode,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt,
		Usage: usageView{
			InputTokens: r.Usage.InputTokens, OutputTokens: r.Usage.OutputTokens, CacheReadTokens: r.Usage.CacheReadTokens,
			CacheWriteTokens: r.Usage.CacheWriteTokens, Estimated: r.Usage.Estimated,
		},
	}
	if micros, priced := run.Cost(r.Usage, r.Prices); priced {
		v.CostMicros = &micros
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// SECURITY: the caller's actor must be the thread's bound actor; anything else reads exactly like a missing thread.
func (h *Handler) boundThread(c caller, actor string, th *run.Thread) error {
	endUser, found, err := h.d.EndUsers.FindAPIUser(c.ctx, c.agent.ProjectID, actor)
	if err != nil {
		return err
	}
	if !found || th.AgentID != c.agent.ID || th.ActorEndUserID == uuid.Nil || th.ActorEndUserID != endUser {
		return &run.ThreadActorMismatchError{ThreadID: th.ID.String()}
	}
	return nil
}

// SECURITY: the actor header is checked before any lookup, so a bad header answers the same whether the run exists.
func (h *Handler) boundRun(c caller, r *http.Request) (*run.Run, *run.Thread, error) {
	actor, err := h.actorValue(c.ctx, r, c.principal)
	if err != nil {
		return nil, nil, err
	}
	found, err := h.d.Store.RunByPublicID(c.ctx, r.PathValue("run"))
	if err != nil {
		if run.IsNotFoundError(err) {
			return nil, nil, errRunNotFound
		}
		return nil, nil, err
	}
	th, err := h.d.Store.Thread(c.ctx, found.ThreadID)
	if err != nil {
		return nil, nil, err
	}
	if err := h.boundThread(c, actor, th); err != nil {
		if run.IsThreadNotFoundError(err) {
			return nil, nil, errRunNotFound
		}
		return nil, nil, err
	}
	return found, th, nil
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	c, err := h.authorize(r, scope.RunRead)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	found, th, err := h.boundRun(c, r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(found, th.ExternalKey))
}

func (h *Handler) cancelRun(w http.ResponseWriter, r *http.Request) {
	c, err := h.authorize(r, scope.RunCreate)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	found, th, err := h.boundRun(c, r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.d.Canceller.Cancel(c.ctx, found.ID); err != nil {
		h.writeError(w, r, err)
		return
	}
	if found, err = h.d.Store.RunByID(c.ctx, found.ID); err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, viewOf(found, th.ExternalKey))
}

type messageView struct {
	Seq     int64           `json:"seq"`
	Status  string          `json:"status"`
	Message json.RawMessage `json:"message"`
}

type messagesPage struct {
	Messages []messageView `json:"messages"`
	Next     int64         `json:"next_after"`
}

func (h *Handler) getMessages(w http.ResponseWriter, r *http.Request) {
	c, err := h.authorize(r, scope.RunRead)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	after, limit, err := pageOf(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	actor, err := h.actorValue(c.ctx, r, c.principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	th, err := h.d.Store.ThreadByExternalKey(c.ctx, c.agent.ID, r.PathValue("thread"))
	if err == nil {
		err = h.boundThread(c, actor, th)
	}
	if err != nil {
		if run.IsThreadNotFoundError(err) {
			err = errThreadNotFound
		}
		h.writeError(w, r, err)
		return
	}
	msgs, err := h.d.Store.ThreadMessages(c.ctx, th.ID, after, limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	page := messagesPage{Messages: make([]messageView, 0, len(msgs)), Next: after}
	for _, m := range msgs {
		page.Messages = append(page.Messages, messageView{Seq: m.Seq, Status: string(m.Status), Message: m.AGUIBody()})
		page.Next = m.Seq
	}
	writeJSON(w, http.StatusOK, page)
}

func pageOf(r *http.Request) (after int64, limit int, err error) {
	after, limit = 0, defaultPage
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, invalid("after", apperror.FieldInvalid)
		}
		after = n
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPage {
			return 0, 0, invalid("limit", apperror.FieldInvalid)
		}
		limit = n
	}
	return after, limit, nil
}
