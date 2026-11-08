package dataplane

import (
	"net/http"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

var errNoSession = apperror.New(apperror.CodeUnauthenticated, "Sign in to try the agent")

const threadKindTry = "try"

// TryRoutes are the console Try it routes keyed "METHOD /path"; the caller mounts them behind the session and cross-origin checks.
func (h *Handler) TryRoutes() map[string]http.Handler {
	return map[string]http.Handler{
		"POST /agents/{agent}/try/runs":                     http.HandlerFunc(h.postTryRun),
		"POST /agents/{agent}/try/runs/{run}/cancel":        http.HandlerFunc(h.cancelTryRun),
		"GET /agents/{agent}/try/threads/{thread}/messages": http.HandlerFunc(h.getTryMessages),
	}
}

// SECURITY: the signed-in member acts as person:<their public id>; an unreached agent is not found, a missing permission forbidden.
func (h *Handler) authorizeMember(r *http.Request, permission string) (caller, string, error) {
	if err := h.d.Limits.ip(r); err != nil {
		return caller{}, "", err
	}
	p := session.PrincipalFrom(r.Context())
	if !p.IsPerson() {
		return caller{}, "", errNoSession
	}
	if err := h.d.Limits.person(r.Context(), p.UserID.String()); err != nil {
		return caller{}, "", err
	}
	c, err := h.reachAgent(r.Context(), r, p, permission)
	if err != nil {
		return caller{}, "", err
	}
	id, err := h.d.Names.UserPublicID(c.ctx, p.UserID)
	if err != nil {
		return caller{}, "", err
	}
	return c, "person:" + id, nil
}

func (h *Handler) postTryRun(w http.ResponseWriter, r *http.Request) {
	c, actor, err := h.authorizeMember(r, scope.RunCreate)
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
	if err := h.d.Drafts.Pin(c.ctx, c.agent.ID, c.principal.UserID); err != nil {
		h.writeError(w, r, err)
		return
	}
	stream, err := h.open(c, in, actor, "", h.d.Store.UpsertTryThread)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.relay(w, r, stream, in.ThreadID)
}

func (h *Handler) cancelTryRun(w http.ResponseWriter, r *http.Request) {
	c, actor, err := h.authorizeMember(r, scope.RunCreate)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	found, th, err := h.actorRun(c, actor, r.PathValue("run"), threadKindTry)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.cancel(w, r, c, found, th)
}

func (h *Handler) getTryMessages(w http.ResponseWriter, r *http.Request) {
	c, actor, err := h.authorizeMember(r, scope.RunRead)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	after, limit, err := pageOf(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	th, err := h.d.Store.TryThreadByExternalKey(c.ctx, c.agent.ID, r.PathValue("thread"))
	h.messages(w, r, c, actor, th, err, after, limit)
}
