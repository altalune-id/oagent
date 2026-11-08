package boot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/runtime"
	rthttp "altalune.id/oagent/internal/kernel/runtime/http"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
)

type runParts struct {
	store  run.Store
	svc    *run.Service
	runner *run.Runner
	kicker run.Kicker
	ready  bool
}

func buildRun(k kernel.Kernel, cfg config.Config, s *Services, limiter ratelimit.Limiter, log *slog.Logger) (runParts, error) {
	store := run.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix).
		WithMaxConcurrentPerProject(cfg.Run.MaxConcurrentPerProject).
		WithMaxMessageAge(cfg.Channel.MaxMessageAge)
	svc := run.NewService(store, db.NewTransactor(k.Pool), k.Reporter.Unexpected).
		WithAgents(runAgents{store: s.AgentStore, agents: s.Agents}).
		WithProviders(runProviders{providers: s.ModelProviders}).
		WithMCPServers(runServers{servers: s.MCPServers}).
		WithSnapshots(snapshotstore.NewPostgres(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)).
		WithPublisher(s.Webhooks).
		WithRateCap(run.RateCap{Limiter: limiter, PerMinute: cfg.Run.MaxPerMinutePerProject}).
		WithTurnTimeout(cfg.Run.TurnTimeout).
		WithMaxAttempts(cfg.Run.MaxAttempts).
		WithLogger(orDiscard(log))
	if s.EndUsers != nil {
		svc.WithNames(endUserNames{users: s.EndUsers})
	}
	rt, err := buildRuntime(cfg.Runtime)
	if err != nil {
		return runParts{}, err
	}
	if rt != nil {
		svc.WithRuntime(rt)
	} else if log != nil {
		log.Warn("boot: runtime.url is empty; agent messages wait pending until a runtime is configured")
	}
	runner := run.NewRunner(svc, store, run.RunnerOptions{
		MaxConcurrent: cfg.Run.MaxConcurrentPerInstance, LeaseTTL: cfg.Run.LeaseTTL, Heartbeat: cfg.Run.Heartbeat,
		DrainTimeout: cfg.Run.DrainTimeout, TurnTimeout: cfg.Run.TurnTimeout,
	}).WithLogger(orDiscard(log)).WithWindows(store)
	if s.Channels != nil {
		svc.WithMedia(channel.Media{Service: s.Channels, Provider: s.ChannelProvider})
		s.ChannelReplier = channel.NewReplier(s.Channels, store, s.ChannelProvider, orDiscard(log))
		runner.WithReplies(s.ChannelReplier)
	}
	kicker := run.NewKicker(k.Queue, cfg.Queue.Enabled, runner)
	runner.WithKicker(kicker)
	return runParts{store: store, svc: svc, runner: runner, kicker: kicker, ready: rt != nil}, nil
}

func buildRuntime(c config.RuntimeConfig) (runtime.Runtime, error) {
	if c.URL == "" {
		return nil, nil
	}
	switch c.Driver {
	case config.RuntimeDriverHTTP:
		rt, err := rthttp.New(runtime.Config{Driver: string(c.Driver), URL: c.URL, Token: c.Token})
		if err != nil {
			return nil, fmt.Errorf("boot: runtime: %w", err)
		}
		return rt, nil
	default:
		return nil, fmt.Errorf("boot: runtime.driver %q has no driver", c.Driver)
	}
}

type runAgents struct {
	store  agent.Store
	agents *agent.Service
}

func (a runAgents) Version(ctx context.Context, id uuid.UUID) (*run.AgentVersion, error) {
	v, err := a.store.VersionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.pinned(ctx, v)
}

func (a runAgents) Latest(ctx context.Context, agentID uuid.UUID) (*run.AgentVersion, error) {
	v, err := a.store.LatestVersion(ctx, agentID)
	if err != nil || v == nil {
		return nil, err
	}
	return a.pinned(ctx, v)
}

func (a runAgents) Draft(ctx context.Context, agentID uuid.UUID) (*run.AgentVersion, error) {
	v, err := a.agents.DraftVersion(ctx, agentID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return a.pinned(ctx, v)
}

func (a runAgents) pinned(ctx context.Context, v *agent.Version) (*run.AgentVersion, error) {
	ag, err := a.store.ByID(ctx, v.AgentID)
	if err != nil {
		return nil, err
	}
	c := v.Config
	out := &run.AgentVersion{
		ID: v.ID, AgentID: v.AgentID, AgentPublicID: ag.PublicID, Version: v.Version, Draft: v.Kind == agent.VersionDraft, Legacy: c.Legacy(),
		Instructions: c.Instructions, Model: c.Model, Temperature: c.Temperature, MaxSteps: c.MaxSteps,
		MaxCostCents: c.MaxCostCents, ProviderID: c.ProviderID,
	}
	for _, g := range c.MCPServers {
		grant := run.MCPGrant{ServerID: g.ServerID}
		for _, t := range g.Tools {
			grant.Tools = append(grant.Tools, run.ToolGrant{Name: t.Name, RequireApproval: t.RequireApproval})
		}
		out.MCPServers = append(out.MCPServers, grant)
	}
	return out, nil
}

type runProviders struct{ providers *modelprovider.Service }

func (p runProviders) ResolveModel(ctx context.Context, providerID uuid.UUID, model string) (run.ResolvedModel, error) {
	r, err := p.providers.ResolveModel(ctx, providerID, model)
	if err != nil {
		return run.ResolvedModel{}, err
	}
	out := run.ResolvedModel{BaseURL: r.BaseURL, APIKey: r.APIKey, Vision: r.Model.Vision}
	if pr := r.Model.Prices; pr != nil {
		out.Prices = &run.Prices{
			Input: new(pr.InputMicros), Output: new(pr.OutputMicros),
			CacheRead: new(pr.CacheReadMicros), CacheWrite: new(pr.CacheWriteMicros),
		}
	}
	return out, nil
}

type runServers struct{ servers *mcpserver.Service }

func (s runServers) Open(ctx context.Context, id uuid.UUID) (run.OpenedServer, error) {
	srv, value, err := s.servers.Open(ctx, id)
	if err != nil {
		return run.OpenedServer{}, err
	}
	return run.OpenedServer{
		Name: srv.Name, URL: srv.URL, AuthKind: string(srv.AuthKind), AuthHeader: srv.AuthHeader, AuthValue: value,
		TimeoutMS: srv.TimeoutMS,
	}, nil
}
