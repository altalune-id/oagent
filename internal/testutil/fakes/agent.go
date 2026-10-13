package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/agent"
)

// AgentStore is an agent.Store test double. ByID/List return Agent/Agents (or Err); Save/Delete record and return SaveFn/DeleteFn/Err.
type AgentStore struct {
	Err      error
	Agent    *agent.Agent
	Agents   []*agent.Agent
	SaveFn   func(*agent.Agent) error
	Saved    *agent.Agent
	DeleteFn func(uuid.UUID) error
	Deleted  uuid.UUID
}

var _ agent.Store = (*AgentStore)(nil)

// Save implements agent.Store.
func (f *AgentStore) Save(_ context.Context, a *agent.Agent) error {
	f.Saved = a
	if f.SaveFn != nil {
		return f.SaveFn(a)
	}
	return f.Err
}

// Delete implements agent.Store.
func (f *AgentStore) Delete(_ context.Context, id uuid.UUID) error {
	f.Deleted = id
	if f.DeleteFn != nil {
		return f.DeleteFn(id)
	}
	return f.Err
}

// ByID implements agent.Store.
func (f *AgentStore) ByID(_ context.Context, id uuid.UUID) (*agent.Agent, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Agent == nil {
		return nil, &agent.NotFoundError{ID: id.String()}
	}
	return f.Agent, nil
}

// ByPublicID implements agent.Store; it finds Agent or one of Agents by public id.
func (f *AgentStore) ByPublicID(_ context.Context, publicID string) (*agent.Agent, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	for _, a := range append([]*agent.Agent{f.Agent}, f.Agents...) {
		if a != nil && a.PublicID == publicID {
			return a, nil
		}
	}
	return nil, &agent.NotFoundError{ID: publicID}
}

// List implements agent.Store.
func (f *AgentStore) List(context.Context, uuid.UUID) ([]*agent.Agent, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Agents, nil
}
