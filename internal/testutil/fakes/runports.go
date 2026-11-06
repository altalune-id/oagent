package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/run"
)

var errUnknownRef = errors.New("fakes: unknown id")

// RunAgents is an in-memory run.Agents.
type RunAgents struct {
	Versions map[uuid.UUID]*run.AgentVersion
}

var _ run.Agents = (*RunAgents)(nil)

// Version implements run.Agents.
func (f *RunAgents) Version(_ context.Context, id uuid.UUID) (*run.AgentVersion, error) {
	v, ok := f.Versions[id]
	if !ok {
		return nil, errUnknownRef
	}
	return v, nil
}

// Latest implements run.Agents: the highest version of agentID, nil when there is none.
func (f *RunAgents) Latest(_ context.Context, agentID uuid.UUID) (*run.AgentVersion, error) {
	var latest *run.AgentVersion
	for _, v := range f.Versions {
		if v.AgentID == agentID && (latest == nil || v.Version > latest.Version) {
			latest = v
		}
	}
	return latest, nil
}

// RunProviders is an in-memory run.Providers keyed by provider and model.
type RunProviders struct {
	Models map[uuid.UUID]map[string]run.ResolvedModel
	Err    error
}

var _ run.Providers = (*RunProviders)(nil)

// ResolveModel implements run.Providers.
func (f *RunProviders) ResolveModel(_ context.Context, providerID uuid.UUID, model string) (run.ResolvedModel, error) {
	if f.Err != nil {
		return run.ResolvedModel{}, f.Err
	}
	m, ok := f.Models[providerID][model]
	if !ok {
		return run.ResolvedModel{}, errUnknownRef
	}
	return m, nil
}

// RunMCPServers is an in-memory run.MCPServers; Opened records each server opened.
type RunMCPServers struct {
	mu      sync.Mutex
	Servers map[uuid.UUID]run.OpenedServer
	Opened  []uuid.UUID
}

var _ run.MCPServers = (*RunMCPServers)(nil)

// Open implements run.MCPServers.
func (f *RunMCPServers) Open(_ context.Context, id uuid.UUID) (run.OpenedServer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Servers[id]
	if !ok {
		return run.OpenedServer{}, errUnknownRef
	}
	f.Opened = append(f.Opened, id)
	return s, nil
}
