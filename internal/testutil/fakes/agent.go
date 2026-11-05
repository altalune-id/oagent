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
	// Published holds the inserted versions, newest first.
	Published []*agent.Version
	// Usage and ServerInUse answer the usage queries.
	Usage       agent.ProviderUsage
	ServerInUse bool
}

// SaveDraft implements agent.Store like Save.
func (f *AgentStore) SaveDraft(ctx context.Context, a *agent.Agent) error { return f.Save(ctx, a) }

// MarkPublished implements agent.Store like Save.
func (f *AgentStore) MarkPublished(ctx context.Context, a *agent.Agent) error { return f.Save(ctx, a) }

// LockForPublish implements agent.Store like ByID.
func (f *AgentStore) LockForPublish(ctx context.Context, id uuid.UUID) (*agent.Agent, error) {
	return f.ByID(ctx, id)
}

// InsertVersion implements agent.Store.
func (f *AgentStore) InsertVersion(_ context.Context, v *agent.Version) error {
	if f.Err != nil {
		return f.Err
	}
	f.Published = append([]*agent.Version{v}, f.Published...)
	return nil
}

// Versions implements agent.Store.
func (f *AgentStore) Versions(_ context.Context, _ uuid.UUID, limit int) ([]*agent.Version, error) {
	return f.Published[:min(limit, len(f.Published))], f.Err
}

// LatestVersion implements agent.Store.
func (f *AgentStore) LatestVersion(context.Context, uuid.UUID) (*agent.Version, error) {
	if f.Err != nil || len(f.Published) == 0 {
		return nil, f.Err
	}
	return f.Published[0], nil
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

func (f *AgentStore) byPublicID(publicID string) (*agent.Agent, error) {
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

// Resolve implements agent.Store; it names the agent with publicID.
func (f *AgentStore) Resolve(_ context.Context, publicID string) (agent.Ref, error) {
	p, err := f.byPublicID(publicID)
	if err != nil {
		return agent.Ref{}, err
	}
	return agent.Ref{ID: p.ID, OrgID: p.OrgID, ProjectID: p.ProjectID}, nil
}

// ProviderUse implements agent.Store; Usage answers it.
func (f *AgentStore) ProviderUse(context.Context, uuid.UUID) (agent.ProviderUsage, error) {
	return f.Usage, f.Err
}

// MCPServerInUse implements agent.Store; ServerInUse answers it.
func (f *AgentStore) MCPServerInUse(context.Context, uuid.UUID) (bool, error) {
	return f.ServerInUse, f.Err
}
