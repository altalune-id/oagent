package boot

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/memory"
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

// SECURITY: a grant ref must sit in the caller's project and in the org of the tenant in ctx, on top of RLS.
func inScope(ctx context.Context, refOrg, refProject, projectID uuid.UUID) bool {
	return refProject == projectID && refOrg == tenant.Peek(ctx).OrgID
}

type grantProviders struct{ providers *modelprovider.Service }

// SECURITY: an unknown, foreign or deleted provider answers the agent's grant error, never the provider's own not-found.
func (g grantProviders) Models(ctx context.Context, projectID, providerID uuid.UUID) ([]agent.ModelRef, error) {
	p, err := g.providers.Get(ctx, providerID)
	if modelprovider.IsNotFoundError(err) || (err == nil && !inScope(ctx, p.OrgID, p.ProjectID, projectID)) {
		return nil, &agent.InvalidGrantError{Reason: agent.GrantProvider}
	}
	if err != nil {
		return nil, err
	}
	out := make([]agent.ModelRef, 0, len(p.Models))
	for _, m := range p.Models {
		out = append(out, agent.ModelRef{ID: m.ID, Priced: m.Prices != nil, Transcription: m.Transcription})
	}
	return out, nil
}

type grantServers struct{ servers *mcpserver.Service }

// Exists implements agent.MCPServers; an unknown or foreign server answers the agent's grant error.
func (g grantServers) Exists(ctx context.Context, projectID, serverID uuid.UUID) (string, error) {
	s, err := g.servers.Get(ctx, serverID)
	if mcpserver.IsNotFoundError(err) || (err == nil && !inScope(ctx, s.OrgID, s.ProjectID, projectID)) {
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

type providerUsage struct {
	agents    agent.Store
	knowledge *knowledge.Service
}

// ProviderUse implements modelprovider.Usage over the agents' latest published versions and the project's embedding model.
func (u providerUsage) ProviderUse(ctx context.Context, providerID uuid.UUID) (modelprovider.ProviderUse, error) {
	use, err := u.agents.ProviderUse(ctx, providerID)
	if err != nil {
		return modelprovider.ProviderUse{}, err
	}
	out := modelprovider.ProviderUse{InUse: use.InUse, Models: use.Models, TranscriptionModels: use.TranscriptionModels}
	if u.knowledge != nil {
		if out.EmbeddingModel, err = u.knowledge.EmbeddingModelOf(ctx, providerID); err != nil {
			return modelprovider.ProviderUse{}, err
		}
	}
	return out, nil
}

type grantKnowledge struct{ knowledge *knowledge.Service }

// SourceExists implements agent.Knowledge; an unknown or foreign source answers the agent's grant error.
func (g grantKnowledge) SourceExists(ctx context.Context, projectID, sourceID uuid.UUID) error {
	return knowledgeGrantErr(g.knowledge.SourceExists(ctx, projectID, sourceID))
}

// LockSourcesShared implements agent.Knowledge; a gone source answers the agent's grant error.
func (g grantKnowledge) LockSourcesShared(ctx context.Context, ids []uuid.UUID) error {
	return knowledgeGrantErr(g.knowledge.LockSourcesShared(ctx, ids))
}

func knowledgeGrantErr(err error) error {
	if knowledge.IsSourceNotFoundError(err) {
		return &agent.InvalidGrantError{Reason: agent.GrantKnowledgeSource}
	}
	return err
}

type grantMemory struct{ memory *memory.Service }

// StoreUsable implements agent.Memory; an unknown, foreign or disabled store answers the agent's grant error.
func (g grantMemory) StoreUsable(ctx context.Context, projectID, storeID uuid.UUID) error {
	return memoryGrantErr(g.memory.StoreUsable(ctx, projectID, storeID))
}

// LockStoreShared implements agent.Memory; a gone store answers the agent's grant error.
func (g grantMemory) LockStoreShared(ctx context.Context, id uuid.UUID) error {
	return memoryGrantErr(g.memory.LockStoreShared(ctx, id))
}

func memoryGrantErr(err error) error {
	switch {
	case memory.IsStoreNotFoundError(err):
		return &agent.InvalidGrantError{Reason: agent.GrantMemoryStore}
	case memory.IsInvalidStoreError(err):
		return &agent.InvalidGrantError{Reason: agent.GrantMemoryStoreDisabled}
	}
	return err
}

type knowledgeBindings struct{ agents agent.Store }

// SourceInUse implements knowledge.Bindings over the agents' latest published versions.
func (b knowledgeBindings) SourceInUse(ctx context.Context, sourceID uuid.UUID) (bool, error) {
	return b.agents.KnowledgeSourceInUse(ctx, sourceID)
}

// SourcesUsedBy implements knowledge.Bindings over the agents' latest published versions.
func (b knowledgeBindings) SourcesUsedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]knowledge.UsedBy, error) {
	use, err := b.agents.KnowledgeUse(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]knowledge.UsedBy, len(use))
	for id, agents := range use {
		for _, a := range agents {
			out[id] = append(out[id], knowledge.UsedBy{AgentID: a.ID, AgentPublicID: a.PublicID, AgentName: a.Name})
		}
	}
	return out, nil
}

type serverUsage struct{ agents agent.Store }

// ServerInUse implements mcpserver.Usage over the agents' latest published versions.
func (u serverUsage) ServerInUse(ctx context.Context, serverID uuid.UUID) (bool, error) {
	return u.agents.MCPServerInUse(ctx, serverID)
}
