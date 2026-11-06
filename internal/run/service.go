package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	defaultTurnTimeout = 10 * time.Minute
	deadlineMargin     = 30 * time.Second
	defaultMaxAttempts = 3
	connectTimeoutMs   = 10_000
	cancelLease        = 30 * time.Second
	noRuntimeWarnEvery = 10 * time.Minute
	charsPerToken      = 4
	expiredToolResult  = "This tool call expired: its approval is no longer available. Ask again if it is still needed."
)

// ErrCancelRequested is the cause a turn's context is cancelled with when someone asked to cancel the run.
var ErrCancelRequested = errors.New("run: cancel requested")

// Service runs agent turns on leased threads.
type Service struct {
	store       Store
	uow         db.Transactor
	unexpected  apperror.UnexpectedFunc
	rt          runtime.Runtime
	agents      Agents
	providers   Providers
	servers     MCPServers
	snaps       SnapshotStore
	publisher   Publisher
	rate        RateCap
	log         *slog.Logger
	turnTimeout time.Duration
	maxAttempts int
	// noRuntimeWarned holds the last no-runtime warning per thread, so a reconcile tick does not log it every time.
	noRuntimeWarned sync.Map
}

// NewService builds a Service over store; the With methods add the runtime and the lookups a turn needs.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{
		store: store, uow: uow, unexpected: unexpected, log: slog.Default(),
		turnTimeout: defaultTurnTimeout, maxAttempts: defaultMaxAttempts,
	}
}

// WithRuntime runs turns on rt.
func (s *Service) WithRuntime(rt runtime.Runtime) *Service { s.rt = rt; return s }

// WithAgents loads pinned versions from a.
func (s *Service) WithAgents(a Agents) *Service { s.agents = a; return s }

// WithProviders resolves models through p.
func (s *Service) WithProviders(p Providers) *Service { s.providers = p; return s }

// WithMCPServers opens MCP servers through m.
func (s *Service) WithMCPServers(m MCPServers) *Service { s.servers = m; return s }

// WithSnapshots keeps thread snapshots in st.
func (s *Service) WithSnapshots(st SnapshotStore) *Service { s.snaps = st; return s }

// WithPublisher emits run.completed and run.failed through p, inside the settle's transaction.
func (s *Service) WithPublisher(p Publisher) *Service { s.publisher = p; return s }

// WithRateCap limits how often a project starts a run.
func (s *Service) WithRateCap(c RateCap) *Service { s.rate = c; return s }

// WithLogger logs turn outcomes to l.
func (s *Service) WithLogger(l *slog.Logger) *Service { s.log = l; return s }

// WithTurnTimeout sets the turn deadline; the runtime gets d less a 30 s margin.
func (s *Service) WithTurnTimeout(d time.Duration) *Service {
	if d > deadlineMargin {
		s.turnTimeout = d
	}
	return s
}

// WithMaxAttempts fails a run that was interrupted n times.
func (s *Service) WithMaxAttempts(n int) *Service {
	if n > 0 {
		s.maxAttempts = n
	}
	return s
}

type turn struct {
	run     *Run
	thread  *Thread
	version *AgentVersion
	model   ResolvedModel
	taken   []Message
	token   string
	// stored holds the tool calls whose result this run already stored, from earlier attempts and this one.
	stored map[string]bool
}

// Execute runs the thread's next turn under token's lease: it resumes or starts a run; nil, nil means nothing was pending.
func (s *Service) Execute(ctx context.Context, threadID uuid.UUID, token string) (*Run, error) {
	th, err := s.store.Thread(ctx, threadID)
	if err != nil {
		return nil, s.errOrReport(ctx, "run: thread", err)
	}
	if s.rt == nil {
		s.warnNoRuntime(ctx, threadID)
		return nil, &RuntimeUnavailableError{}
	}
	t, done, err := s.open(ctx, th, token)
	if err != nil || done != nil || t == nil {
		return done, err
	}
	code, err := s.refusal(ctx, t)
	if err != nil {
		return s.abandon(ctx, t, t.run.Usage, err)
	}
	if code != "" {
		return s.fail(ctx, t, code)
	}
	req, history, err := s.request(ctx, t)
	if err != nil {
		return s.abandon(ctx, t, t.run.Usage, err)
	}
	return s.stream(ctx, t, req, history)
}

func (s *Service) abandon(ctx context.Context, t *turn, u Usage, err error) (*Run, error) {
	if IsLeaseLostError(err) {
		return nil, err
	}
	if code := domainCode(err); code != "" {
		return s.fail(ctx, t, code)
	}
	s.log.WarnContext(ctx, "run: turn stopped; the run waits for a retry", "run", t.run.PublicID, "err", err.Error())
	if err := t.run.Move(StatusInterrupted, time.Now()); err != nil {
		return nil, s.errOrReport(ctx, "run: interrupt", err)
	}
	return s.settle(ctx, t, u, nil, nil)
}

func domainCode(err error) apperror.Code {
	if ae, ok := apperror.AsAppError(err); ok && ae.Code() != apperror.CodeUnexpected {
		return ae.Code()
	}
	return ""
}

// Cancel stops the thread's run: a live turn is asked to stop at its next heartbeat, an idle open run is settled cancelled at once.
func (s *Service) Cancel(ctx context.Context, threadID uuid.UUID) error {
	th, err := s.store.Thread(ctx, threadID)
	if err != nil {
		return s.errOrReport(ctx, "run: cancel thread", err)
	}
	token := uuid.NewString()
	claimed, err := s.store.Claim(ctx, threadID, token, cancelLease)
	if err != nil {
		return s.errOrReport(ctx, "run: cancel claim", err)
	}
	if !claimed {
		return s.errOrReport(ctx, "run: request cancel", s.store.RequestCancel(ctx, threadID))
	}
	defer func() {
		if err := s.store.Release(context.WithoutCancel(ctx), threadID, token); err != nil && !IsLeaseLostError(err) {
			s.log.ErrorContext(ctx, "run: cancel release", "thread", threadID.String(), "err", err.Error())
		}
	}()
	r, err := s.store.OpenRun(ctx, threadID)
	if err != nil || r == nil {
		return s.errOrReport(ctx, "run: cancel open run", err)
	}
	t := &turn{run: r, thread: th, token: token, stored: map[string]bool{}}
	if t.version, err = s.agents.Version(ctx, r.AgentVersionID); err != nil {
		return s.errOrReport(ctx, "run: cancel version", err)
	}
	if err := r.Move(StatusCancelled, time.Now()); err != nil {
		return s.errOrReport(ctx, "run: cancel", err)
	}
	_, err = s.settle(ctx, t, r.Usage, nil, nil)
	return err
}

func (s *Service) open(ctx context.Context, th *Thread, token string) (*turn, *Run, error) {
	r, err := s.store.OpenRun(ctx, th.ID)
	if err != nil {
		return nil, nil, s.errOrReport(ctx, "run: open run", err)
	}
	t := &turn{run: r, thread: th, token: token, stored: map[string]bool{}}
	if r != nil {
		return s.resume(ctx, t)
	}
	pending, err := s.store.HasPending(ctx, th.ID)
	if err != nil || !pending {
		return nil, nil, s.errOrReport(ctx, "run: pending", err)
	}
	if t.version, err = s.agents.Latest(ctx, th.AgentID); err != nil {
		return nil, nil, s.errOrReport(ctx, "run: latest version", err)
	}
	if t.version == nil {
		s.log.DebugContext(ctx, "run: the agent has no published version; the messages wait", "thread", th.ID.String())
		return nil, nil, nil
	}
	if err := s.rate.Allow(ctx, th.ProjectID); err != nil {
		return nil, nil, s.errOrReport(ctx, "run: rate", err)
	}
	t.run = New(th.OrgID, th.ProjectID, th.ID, t.version.ID, time.Now())
	if !t.version.Legacy {
		if m, err := s.providers.ResolveModel(ctx, t.version.ProviderID, t.version.Model); err == nil {
			t.model, t.run.Prices = m, m.Prices
		}
	}
	t.taken, err = s.store.StartRun(ctx, token, t.run)
	if errors.Is(err, ErrNothingPending) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, s.errOrReport(ctx, "run: start", err)
	}
	return t, nil, nil
}

func (s *Service) resume(ctx context.Context, t *turn) (*turn, *Run, error) {
	r := t.run
	var err error
	if t.version, err = s.agents.Version(ctx, r.AgentVersionID); err != nil {
		return nil, nil, s.errOrReport(ctx, "run: pinned version", err)
	}
	if t.thread.CancelRequested {
		if err := r.Move(StatusCancelled, time.Now()); err != nil {
			return nil, nil, s.errOrReport(ctx, "run: cancel", err)
		}
		done, err := s.settle(ctx, t, r.Usage, nil, nil)
		return nil, done, err
	}
	if int(r.Attempt) >= s.maxAttempts {
		done, err := s.fail(ctx, t, apperror.CodeRunAttemptsExhausted)
		return nil, done, err
	}
	if err := s.rate.Allow(ctx, t.thread.ProjectID); err != nil {
		return nil, nil, s.errOrReport(ctx, "run: rate", err)
	}
	if err := r.Move(StatusRunning, time.Now()); err != nil {
		return nil, nil, s.errOrReport(ctx, "run: resume", err)
	}
	t.taken, err = s.store.StartRun(ctx, t.token, r)
	if errors.Is(err, ErrNothingPending) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, s.errOrReport(ctx, "run: resume", err)
	}
	return t, nil, nil
}

func (s *Service) refusal(ctx context.Context, t *turn) (apperror.Code, error) {
	v := t.version
	if v.Legacy {
		return apperror.CodeAgentNeedsRepublish, nil
	}
	m, err := s.providers.ResolveModel(ctx, v.ProviderID, v.Model)
	if err != nil {
		return "", fmt.Errorf("run: resolve model: %w", err)
	}
	if strings.ContainsFunc(m.BaseURL, unicode.IsControl) {
		return apperror.CodeModelProviderInvalidBaseURL, nil
	}
	if strings.TrimSpace(m.APIKey) == "" || strings.ContainsFunc(m.APIKey, unicode.IsControl) {
		return apperror.CodeModelProviderSecretRequired, nil
	}
	t.model = m
	if v.MaxCostCents > 0 && t.run.Prices == nil {
		return apperror.CodeAgentInvalidGrant, nil
	}
	return "", nil
}

func (s *Service) fail(ctx context.Context, t *turn, code apperror.Code) (*Run, error) {
	if err := t.run.Move(StatusFailed, time.Now()); err != nil {
		return nil, s.errOrReport(ctx, "run: fail", err)
	}
	t.run.ErrorCode = string(code)
	return s.settle(ctx, t, t.run.Usage, nil, nil)
}

func (s *Service) settle(ctx context.Context, t *turn, u Usage, final []Message, snap *Snapshot) (*Run, error) {
	ctx = context.WithoutCancel(ctx)
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.Settle(ctx, t.token, t.run, u, final); err != nil {
			return err
		}
		if snap != nil && s.snaps != nil {
			if err := s.snaps.Save(ctx, snap); err != nil {
				return err
			}
		}
		return s.emit(ctx, t, u)
	})
	if IsLeaseLostError(err) {
		return nil, err
	}
	if err != nil {
		return nil, &SettleError{Cause: s.errOrReport(ctx, "run: settle", err)}
	}
	return t.run, nil
}

// EventFor is the webhook event a run settled in status s emits: every terminal status emits, nothing else does.
func EventFor(s Status) events.Type {
	switch s {
	case StatusCompleted, StatusLimitReached:
		return events.RunCompleted
	case StatusFailed, StatusCancelled, StatusExpired:
		return events.RunFailed
	default:
		return ""
	}
}

func (s *Service) emit(ctx context.Context, t *turn, u Usage) error {
	r := t.run
	kind := EventFor(r.Status)
	if kind == "" || s.publisher == nil {
		return nil
	}
	payload := events.RunSettledV1{
		RunID: r.PublicID, ThreadID: t.thread.PublicID, AgentID: t.version.AgentPublicID, Status: string(r.Status),
		Usage: events.RunUsageV1{
			InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CacheReadInputTokens: u.CacheReadTokens,
			CacheWriteInputTokens: u.CacheWriteTokens, Estimated: u.Estimated,
		},
	}
	if micros, priced := Cost(u, r.Prices); priced {
		payload.CostMicros = &micros
	}
	var data any = events.RunFailedV1(payload)
	if kind == events.RunCompleted {
		data = events.RunCompletedV1(payload)
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: r.OrgID, ProjectID: r.ProjectID})
	if err := s.publisher.Enqueue(ctx, kind, data); err != nil {
		return fmt.Errorf("run: publish %s: %w", kind, err)
	}
	return nil
}

func (s *Service) warnNoRuntime(ctx context.Context, threadID uuid.UUID) {
	now := time.Now()
	if last, ok := s.noRuntimeWarned.Load(threadID); ok && now.Sub(last.(time.Time)) < noRuntimeWarnEvery {
		return
	}
	s.noRuntimeWarned.Store(threadID, now)
	s.log.WarnContext(ctx, "run: no runtime configured; the thread's messages wait until runtime.url is set", "thread", threadID.String())
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if err == nil || IsExpectedError(err) || IsLeaseLostError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
