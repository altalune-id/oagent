package fakes

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/agent"
)

// GrantProvider is a model provider AgentGrants knows.
type GrantProvider struct {
	ProjectID uuid.UUID
	Models    []agent.ModelRef
}

// GrantServer is an MCP server AgentGrants knows.
type GrantServer struct {
	ProjectID uuid.UUID
	Name      string
}

// AgentGrants implements agent.Providers, agent.MCPServers and agent.Locker over maps; locks need a fake unit of work and are recorded.
type AgentGrants struct {
	Providers map[uuid.UUID]GrantProvider
	Servers   map[uuid.UUID]GrantServer
	// Sources maps a knowledge source to its project.
	Sources map[uuid.UUID]uuid.UUID
	// Stores maps a memory store to its project; DisabledStores are those whose backend is off.
	Stores         map[uuid.UUID]uuid.UUID
	DisabledStores map[uuid.UUID]bool
	// Locked records each LockShared call's provider and server ids; LockedSources and LockedStores the other locks' ids.
	Locked        [][]uuid.UUID
	LockedSources [][]uuid.UUID
	LockedStores  []uuid.UUID
	Err           error
}

var (
	_ agent.Providers  = (*AgentGrants)(nil)
	_ agent.MCPServers = (*AgentGrants)(nil)
	_ agent.Locker     = (*AgentGrants)(nil)
	_ agent.Knowledge  = (*AgentGrants)(nil)
	_ agent.Memory     = (*AgentGrants)(nil)
)

// Models implements agent.Providers.
func (f *AgentGrants) Models(_ context.Context, projectID, providerID uuid.UUID) ([]agent.ModelRef, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	p, ok := f.Providers[providerID]
	if !ok || p.ProjectID != projectID {
		return nil, &agent.InvalidGrantError{Reason: agent.GrantProvider}
	}
	return p.Models, nil
}

// Exists implements agent.MCPServers.
func (f *AgentGrants) Exists(_ context.Context, projectID, serverID uuid.UUID) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	s, ok := f.Servers[serverID]
	if !ok || s.ProjectID != projectID {
		return "", &agent.InvalidGrantError{Reason: agent.GrantServer}
	}
	return s.Name, nil
}

// LockShared implements agent.Locker; a gone provider or server answers *agent.InvalidGrantError.
func (f *AgentGrants) LockShared(ctx context.Context, providerIDs, serverIDs []uuid.UUID) error {
	if !InTx(ctx) {
		return errors.New("fakes: a row lock needs a unit of work")
	}
	if f.Err != nil {
		return f.Err
	}
	for _, id := range providerIDs {
		if _, ok := f.Providers[id]; !ok {
			return &agent.InvalidGrantError{Reason: agent.GrantProvider}
		}
	}
	for _, id := range serverIDs {
		if _, ok := f.Servers[id]; !ok {
			return &agent.InvalidGrantError{Reason: agent.GrantServer}
		}
	}
	f.Locked = append(f.Locked, append(append([]uuid.UUID{}, providerIDs...), serverIDs...))
	return nil
}

// SourceExists implements agent.Knowledge.
func (f *AgentGrants) SourceExists(_ context.Context, projectID, sourceID uuid.UUID) error {
	if f.Err != nil {
		return f.Err
	}
	if p, ok := f.Sources[sourceID]; !ok || p != projectID {
		return &agent.InvalidGrantError{Reason: agent.GrantKnowledgeSource}
	}
	return nil
}

// LockSourcesShared implements agent.Knowledge; a gone source answers *agent.InvalidGrantError.
func (f *AgentGrants) LockSourcesShared(ctx context.Context, ids []uuid.UUID) error {
	if !InTx(ctx) {
		return errors.New("fakes: a row lock needs a unit of work")
	}
	for _, id := range ids {
		if _, ok := f.Sources[id]; !ok {
			return &agent.InvalidGrantError{Reason: agent.GrantKnowledgeSource}
		}
	}
	f.LockedSources = append(f.LockedSources, append([]uuid.UUID{}, ids...))
	return nil
}

// StoreUsable implements agent.Memory.
func (f *AgentGrants) StoreUsable(_ context.Context, projectID, storeID uuid.UUID) error {
	if f.Err != nil {
		return f.Err
	}
	if p, ok := f.Stores[storeID]; !ok || p != projectID {
		return &agent.InvalidGrantError{Reason: agent.GrantMemoryStore}
	}
	if f.DisabledStores[storeID] {
		return &agent.InvalidGrantError{Reason: agent.GrantMemoryStoreDisabled}
	}
	return nil
}

// LockStoreShared implements agent.Memory; it needs a unit of work, like the row lock it stands for.
func (f *AgentGrants) LockStoreShared(ctx context.Context, id uuid.UUID) error {
	if !InTx(ctx) {
		return errors.New("fakes: a row lock needs a unit of work")
	}
	if _, ok := f.Stores[id]; !ok {
		return &agent.InvalidGrantError{Reason: agent.GrantMemoryStore}
	}
	f.LockedStores = append(f.LockedStores, id)
	return nil
}
