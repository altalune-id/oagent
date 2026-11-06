package fakes

import (
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/mcpserver"
)

// MCPServerStore is an in-memory mcpserver.Store; Err fails every call, and locks need a fake unit of work.
type MCPServerStore struct {
	Err     error
	Servers map[uuid.UUID]*mcpserver.Server
	// Locks records "update:<id>" and "share:<id>" for each lock taken.
	Locks []string
}

var _ mcpserver.Store = (*MCPServerStore)(nil)

// Save implements mcpserver.Store; a name taken by another server in the project is an *AlreadyExistsError.
func (f *MCPServerStore) Save(_ context.Context, p *mcpserver.Server) error {
	if f.Err != nil {
		return f.Err
	}
	for _, other := range f.Servers {
		if other.ID != p.ID && other.ProjectID == p.ProjectID && other.Name == p.Name {
			return &mcpserver.AlreadyExistsError{Name: p.Name}
		}
	}
	if f.Servers == nil {
		f.Servers = map[uuid.UUID]*mcpserver.Server{}
	}
	saved := *p
	f.Servers[p.ID] = &saved
	return nil
}

// Delete implements mcpserver.Store.
func (f *MCPServerStore) Delete(_ context.Context, id uuid.UUID) error {
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.Servers[id]; !ok {
		return &mcpserver.NotFoundError{ID: id.String()}
	}
	delete(f.Servers, id)
	return nil
}

// ByID implements mcpserver.Store.
func (f *MCPServerStore) ByID(_ context.Context, id uuid.UUID) (*mcpserver.Server, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	p, ok := f.Servers[id]
	if !ok {
		return nil, &mcpserver.NotFoundError{ID: id.String()}
	}
	out := *p
	return &out, nil
}

// List implements mcpserver.Store; it returns the project's servers oldest first.
func (f *MCPServerStore) List(_ context.Context, projectID uuid.UUID) ([]*mcpserver.Server, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := []*mcpserver.Server{}
	for _, p := range f.Servers {
		if p.ProjectID == projectID {
			cp := *p
			out = append(out, &cp)
		}
	}
	slices.SortFunc(out, func(a, b *mcpserver.Server) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), slices.Compare(a.ID[:], b.ID[:]))
	})
	return out, nil
}

// Resolve implements mcpserver.Store; it names the server with publicID.
func (f *MCPServerStore) Resolve(_ context.Context, publicID string) (mcpserver.Ref, error) {
	if f.Err != nil {
		return mcpserver.Ref{}, f.Err
	}
	for _, p := range f.Servers {
		if p.PublicID == publicID {
			return mcpserver.Ref{ID: p.ID, OrgID: p.OrgID, ProjectID: p.ProjectID}, nil
		}
	}
	return mcpserver.Ref{}, &mcpserver.NotFoundError{}
}

// LockForUpdate implements mcpserver.Store like ByID and records the lock; outside a unit of work it fails.
func (f *MCPServerStore) LockForUpdate(ctx context.Context, id uuid.UUID) (*mcpserver.Server, error) {
	if !InTx(ctx) {
		return nil, errLockOutsideTx
	}
	p, err := f.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	f.Locks = append(f.Locks, "update:"+id.String())
	return p, nil
}

// LockShared implements mcpserver.Store; a missing server is a *NotFoundError, and outside a unit of work it fails.
func (f *MCPServerStore) LockShared(ctx context.Context, ids []uuid.UUID) error {
	if !InTx(ctx) {
		return errLockOutsideTx
	}
	if f.Err != nil {
		return f.Err
	}
	for _, id := range ids {
		if _, ok := f.Servers[id]; !ok {
			return &mcpserver.NotFoundError{ID: id.String()}
		}
	}
	for _, id := range ids {
		f.Locks = append(f.Locks, "share:"+id.String())
	}
	return nil
}
