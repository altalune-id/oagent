package boot

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
)

// configureModelProviders allows http:// base URLs outside production (dev/self-hosted).
func configureModelProviders(svc *modelprovider.Service, cfg config.Config) *modelprovider.Service {
	if cfg.Env.IsProduction() {
		return svc
	}
	return svc.WithAllowInsecureURL()
}

// configureMCPServers lets the probe reach http:// and private hosts outside production only.
func configureMCPServers(svc *mcpserver.Service, cfg config.Config, log *slog.Logger) *mcpserver.Service {
	svc.WithHTTPClient(mcpserver.NewProbeClient(!cfg.Env.IsProduction())).WithLogger(log)
	if cfg.Env.IsProduction() {
		return svc
	}
	return svc.WithAllowInsecureURL()
}

type grantProviders struct{ providers *modelprovider.Service }

// SECURITY: an unknown, foreign or deleted provider answers the agent's grant error, never the provider's own not-found.
func (g grantProviders) Models(ctx context.Context, projectID, providerID uuid.UUID) ([]agent.ModelRef, error) {
	p, err := g.providers.Get(ctx, providerID)
	if modelprovider.IsNotFoundError(err) || (err == nil && p.ProjectID != projectID) {
		return nil, &agent.InvalidGrantError{Reason: agent.GrantProvider}
	}
	if err != nil {
		return nil, err
	}
	out := make([]agent.ModelRef, 0, len(p.Models))
	for _, m := range p.Models {
		out = append(out, agent.ModelRef{ID: m.ID, Priced: m.Prices != nil})
	}
	return out, nil
}

type grantServers struct{ servers *mcpserver.Service }

// Exists implements agent.MCPServers; an unknown or foreign server answers the agent's grant error.
func (g grantServers) Exists(ctx context.Context, projectID, serverID uuid.UUID) (string, error) {
	s, err := g.servers.Get(ctx, serverID)
	if mcpserver.IsNotFoundError(err) || (err == nil && s.ProjectID != projectID) {
		return "", &agent.InvalidGrantError{Reason: agent.GrantServer}
	}
	if err != nil {
		return "", err
	}
	return s.Name, nil
}

type grantLocker struct {
	providers modelprovider.Store
	servers   mcpserver.Store
}

// LockShared implements agent.Locker; a provider or server deleted before the lock answers the agent's grant error.
func (g grantLocker) LockShared(ctx context.Context, providerIDs, serverIDs []uuid.UUID) error {
	if err := g.providers.LockShared(ctx, providerIDs); err != nil {
		if modelprovider.IsNotFoundError(err) {
			return &agent.InvalidGrantError{Reason: agent.GrantProvider}
		}
		return err
	}
	if err := g.servers.LockShared(ctx, serverIDs); err != nil {
		if mcpserver.IsNotFoundError(err) {
			return &agent.InvalidGrantError{Reason: agent.GrantServer}
		}
		return err
	}
	return nil
}

type providerUsage struct{ agents agent.Store }

// ProviderUse implements modelprovider.Usage over the agents' latest published versions.
func (u providerUsage) ProviderUse(ctx context.Context, providerID uuid.UUID) (modelprovider.ProviderUse, error) {
	use, err := u.agents.ProviderUse(ctx, providerID)
	if err != nil {
		return modelprovider.ProviderUse{}, err
	}
	return modelprovider.ProviderUse{InUse: use.InUse, Models: use.Models}, nil
}

type serverUsage struct{ agents agent.Store }

// ServerInUse implements mcpserver.Usage over the agents' latest published versions.
func (u serverUsage) ServerInUse(ctx context.Context, serverID uuid.UUID) (bool, error) {
	return u.agents.MCPServerInUse(ctx, serverID)
}
