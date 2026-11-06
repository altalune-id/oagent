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
	// Locked records each LockShared call's provider and server ids.
	Locked [][]uuid.UUID
	Err    error
}

var (
	_ agent.Providers  = (*AgentGrants)(nil)
	_ agent.MCPServers = (*AgentGrants)(nil)
	_ agent.Locker     = (*AgentGrants)(nil)
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
