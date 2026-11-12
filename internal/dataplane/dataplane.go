package dataplane

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/run"
)

// Keys authenticates an API key credential into the principal it acts as.
type Keys interface {
	Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error)
}

// AgentRef names an agent and the tenant it belongs to; Draft is true while no version is published.
type AgentRef struct {
	ID, OrgID, ProjectID uuid.UUID
	Draft                bool
}

// Agents resolves an agent public id, in any org; found is false for an unknown agent, and the caller checks reach.
type Agents interface {
	Resolve(ctx context.Context, publicID string) (ref AgentRef, found bool, err error)
}

// Names turns a personal key's owner into the public id its actor identity is built from.
type Names interface {
	UserPublicID(ctx context.Context, userID uuid.UUID) (string, error)
}

// EndUsers resolves an api_user identity of a project to its end user; FindAPIUser never creates one.
type EndUsers interface {
	UpsertAPIUser(ctx context.Context, projectID uuid.UUID, value string) (uuid.UUID, error)
	FindAPIUser(ctx context.Context, projectID uuid.UUID, value string) (uuid.UUID, bool, error)
}

// Canceller stops a run, settling it at once when no turn holds its thread.
type Canceller interface {
	Cancel(ctx context.Context, runID uuid.UUID) error
}

// Drafts pins an agent's draft for a console Try it run, refused with the errors a publish gives.
type Drafts interface {
	Pin(ctx context.Context, agentID, by uuid.UUID) error
}

// Runner holds a turn slot for a POST and runs the lease the POST claimed on it.
type Runner interface {
	Reserve() (run.Slot, bool)
	RunHeld(slot run.Slot, k run.Kick, token string, sink *run.Sink) error
}

// Deps is what the data plane needs; RuntimeReady reports whether a runtime is configured.
type Deps struct {
	// Approvals reads a thread's pending approval and decides it for a resume[]; nil turns resume and the 409 off.
	Approvals    Approvals
	Keys         Keys
	KeyPrefix    string
	Agents       Agents
	Names        Names
	EndUsers     EndUsers
	Store        run.Store
	Runner       Runner
	Canceller    Canceller
	Drafts       Drafts
	Limits       Limits
	Rate         run.RateCap
	RuntimeReady func() bool
	LeaseTTL     time.Duration
	Unexpected   apperror.UnexpectedFunc
	Log          *slog.Logger
}

// PendingApproval is a thread's pending approval: Open lists the tool call ids a resume[] must answer.
type PendingApproval struct {
	ID       uuid.UUID
	PublicID string
	Open     []string
}

// Approvals reads a thread's pending approval, nil when none waits, and decides it for the caller on ctx without a kick.
type Approvals interface {
	PendingForThread(ctx context.Context, threadID uuid.UUID) (*PendingApproval, error)
	Decide(ctx context.Context, id uuid.UUID, approved bool, text string) error
}

// Handler serves the /api/v1/ integrator API.
type Handler struct {
	d Deps
}

// New builds the data plane over d.
func New(d Deps) *Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.LeaseTTL <= 0 {
		d.LeaseTTL = time.Minute
	}
	if d.RuntimeReady == nil {
		d.RuntimeReady = func() bool { return true }
	}
	return &Handler{d: d}
}

// Register mounts the data plane routes under basePath on mux. SECURITY: no CORS on any route; it is a server-to-server API.
func (h *Handler) Register(mux *http.ServeMux, basePath string) {
	mux.HandleFunc("POST "+basePath+"/api/v1/agents/{agent}/runs", h.postRuns)
	mux.HandleFunc("GET "+basePath+"/api/v1/agents/{agent}/runs/{run}", h.getRun)
	mux.HandleFunc("POST "+basePath+"/api/v1/agents/{agent}/runs/{run}/cancel", h.cancelRun)
	mux.HandleFunc("GET "+basePath+"/api/v1/agents/{agent}/threads/{thread}/messages", h.getMessages)
}
