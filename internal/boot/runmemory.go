package boot

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/run"
)

type runMemory struct{ memory *memory.Service }

func scopeOf(s run.MemoryScope) memory.Scope {
	return memory.Scope{OrgID: s.OrgID, ProjectID: s.ProjectID, StoreID: s.StoreID, AgentID: s.AgentID, EndUserID: s.EndUserID}
}

// Store implements run.Memory; the memory service answers by the store's backend and opt-in.
func (m runMemory) Store(ctx context.Context, scope run.MemoryScope) (run.StoreMode, error) {
	mode, err := m.memory.Mode(ctx, scopeOf(scope))
	return run.StoreMode{Extracts: mode.Extracts, APIUsers: mode.APIUsers}, err
}

// Recall implements run.Memory over the memory service.
func (m runMemory) Recall(ctx context.Context, scope run.MemoryScope, query string) ([]run.RecalledFact, error) {
	facts, err := m.memory.Recall(ctx, scopeOf(scope), query, runtime.MaxMemoryFacts)
	out := make([]run.RecalledFact, len(facts))
	for i, f := range facts {
		out[i] = run.RecalledFact{Text: f.Text, CreatedAt: f.CreatedAt}
	}
	return out, err
}

// Record implements run.Memory over the memory service.
func (m runMemory) Record(ctx context.Context, scope run.MemoryScope, rec run.MemoryRecord) error {
	return m.memory.Record(ctx, scopeOf(scope), memory.Turn{RunID: rec.RunID, Facts: rec.Facts, UserTexts: rec.UserTexts})
}

type runRememberers struct{ users *enduser.Service }

// Rememberable implements run.Rememberers over the end users' identity classes.
func (r runRememberers) Rememberable(ctx context.Context, endUserID uuid.UUID) (run.Rememberer, error) {
	class, err := r.users.Rememberable(ctx, endUserID)
	return rememberer(class), err
}

func rememberer(class enduser.MemoryClass) run.Rememberer {
	switch class {
	case enduser.MemoryPerson:
		return run.RememberAlways
	case enduser.MemoryNamedAPIUser:
		return run.RememberNamedAPIUser
	default:
		return run.RememberNever
	}
}

type memoryEndUsers struct{ users *enduser.Service }

// Resolve implements memory.EndUsers.
func (m memoryEndUsers) Resolve(ctx context.Context, publicID string) (memory.Ref, bool, error) {
	r, err := m.users.Resolve(ctx, publicID)
	if enduser.IsNotFoundError(err) {
		return memory.Ref{}, false, nil
	}
	return memory.Ref{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID}, err == nil, err
}

type memoryAgents struct {
	agents *agent.Service
	store  agent.Store
}

// Resolve implements memory.Agents.
func (m memoryAgents) Resolve(ctx context.Context, publicID string) (memory.Ref, bool, error) {
	r, err := m.agents.Resolve(ctx, publicID)
	if agent.IsNotFoundError(err) {
		return memory.Ref{}, false, nil
	}
	return memory.Ref{ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID}, err == nil, err
}

// PublicIDs implements memory.Agents.
func (m memoryAgents) PublicIDs(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return m.store.AgentPublicIDs(tenant.WithOrg(ctx, orgID), ids)
}

// NOTE: reads svcs.RunStore at call time; the run store is built after the services.
type memoryRuns struct{ svcs *Services }

// PublicIDs implements memory.Runs; a run that is gone is left out.
func (m memoryRuns) PublicIDs(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return m.svcs.RunStore.RunPublicIDs(tenant.WithOrg(ctx, orgID), ids)
}

func memoryService(svcs *Services, cfg config.Config, uow db.Transactor, unexpected apperror.UnexpectedFunc) *memory.Service {
	return memory.NewService(svcs.MemoryStores, svcs.MemoryDrivers, memoryBackends(cfg), memory.Deps{
		UOW: uow, Users: memoryEndUsers{users: svcs.EndUsers}, Agents: memoryAgents{agents: svcs.Agents, store: svcs.AgentStore},
		Runs: memoryRuns{svcs: svcs},
	}, unexpected).WithBindings(memoryBindings{agents: svcs.AgentStore})
}

type memoryBindings struct{ agents agent.Store }

// StoreInUse implements memory.Bindings over the agents' latest published versions.
func (b memoryBindings) StoreInUse(ctx context.Context, storeID uuid.UUID) (bool, error) {
	return b.agents.MemoryStoreInUse(ctx, storeID)
}

// StoresUsedBy implements memory.Bindings over the agents' latest published versions.
func (b memoryBindings) StoresUsedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]memory.UsedBy, error) {
	use, err := b.agents.MemoryUse(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]memory.UsedBy, len(use))
	for id, named := range use {
		for _, n := range named {
			out[id] = append(out[id], memory.UsedBy{AgentID: n.ID, AgentPublicID: n.PublicID, Name: n.Name})
		}
	}
	return out, nil
}
