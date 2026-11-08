//go:build integration

package boot

import (
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/run"
)

// Limiters is the limiter set BuildLimiters wires.
type Limiters = limiters

// Shared is the store every limited feature counts in, or nil when no rate limit is on.
func (l limiters) Shared() ratelimit.Limiter { return l.shared }

// RunParts is the run graph BuildRun wires.
type RunParts struct {
	Store  run.Store
	Svc    *run.Service
	Runner *run.Runner
	Kicker run.Kicker
	Ready  bool
}

// BuildServices wraps buildServices for the end-to-end tests.
func BuildServices(k kernel.Kernel, cfg config.Config) *Services { return buildServices(k, cfg) }

// BuildLimiters wraps buildLimiters for the end-to-end tests.
func BuildLimiters(cfg config.Config, rdb redis.UniversalClient, log *slog.Logger) (Limiters, error) {
	return buildLimiters(cfg, rdb, log)
}

// BuildRun wraps buildRun for the end-to-end tests.
func BuildRun(k kernel.Kernel, cfg config.Config, s *Services, limiter ratelimit.Limiter, log *slog.Logger) (RunParts, error) {
	p, err := buildRun(k, cfg, s, limiter, log)
	return RunParts{Store: p.store, Svc: p.svc, Runner: p.runner, Kicker: p.kicker, Ready: p.ready}, err
}

// BuildIngest wraps buildIngest for the end-to-end tests.
func BuildIngest(cfg config.Config, svcs *Services, uow db.Transactor, lims Limiters, log *slog.Logger) http.Handler {
	return buildIngest(cfg, svcs, uow, lims, log)
}

// BuildDataPlane wraps buildDataPlane for the end-to-end tests.
func BuildDataPlane(cfg config.Config, svcs *Services, lims Limiters, unexpected apperror.UnexpectedFunc, log *slog.Logger) http.Handler {
	return buildDataPlane(cfg, svcs, lims, unexpected, log)
}

// BuildConsoleTry wraps buildConsoleTry for the end-to-end tests.
func BuildConsoleTry(
	cfg config.Config, svcs *Services, lims Limiters, unexpected apperror.UnexpectedFunc, log *slog.Logger,
) map[string]http.Handler {
	return buildConsoleTry(cfg, svcs, lims, unexpected, log)
}

// RunAgents wraps the run graph's agent adapter.
func RunAgents(s agent.Store) run.Agents { return runAgents{store: s} }

// RunProviders wraps the run graph's model provider adapter.
func RunProviders(p *modelprovider.Service) run.Providers { return runProviders{providers: p} }

// RunServers wraps the run graph's MCP server adapter.
func RunServers(m *mcpserver.Service) run.MCPServers { return runServers{servers: m} }
