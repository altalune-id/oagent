package memory

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
)

// UsedBy is an agent whose latest published version remembers people in a store.
type UsedBy struct {
	AgentID       uuid.UUID
	AgentPublicID string
	Name          string
}

// StoreView is a store with the agents that use it.
type StoreView struct {
	Store  *MemoryStore
	UsedBy []UsedBy
}

// Bindings tells which published agent versions use a store.
type Bindings interface {
	StoreInUse(ctx context.Context, storeID uuid.UUID) (bool, error)
	StoresUsedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]UsedBy, error)
}

// WithBindings makes DeleteStore refuse a store a published agent version uses, and the reads name those agents; nil skips both.
func (s *Service) WithBindings(b Bindings) *Service {
	s.bindings = b
	return s
}

// Backends lists the backends a new store may use.
func (s *Service) Backends() []Backend { return s.enabled }

// ResolveStore maps a store's public id to its ids across tenants; reach is the caller's to check.
func (s *Service) ResolveStore(ctx context.Context, publicID string) (Ref, error) {
	ref, err := s.stores.ResolveStore(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "memory: resolve store", err)
	}
	return ref, nil
}

// CreateStore adds a store to a project of the org on ctx.
func (s *Service) CreateStore(ctx context.Context, projectID uuid.UUID, in StoreInput) (*MemoryStore, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	store, err := NewStore(tc.OrgID, projectID, in, s.enabled, time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.stores.InsertStore(ctx, store); err != nil {
		return nil, s.errOrReport(ctx, "memory: create store", err)
	}
	return store, nil
}

// ListStores returns a project's stores, oldest first, with the agents that use each.
func (s *Service) ListStores(ctx context.Context, projectID uuid.UUID) ([]StoreView, error) {
	stores, err := s.stores.ListStores(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "memory: list stores", err)
	}
	used, err := s.usedBy(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]StoreView, len(stores))
	for i, st := range stores {
		out[i] = StoreView{Store: st, UsedBy: used[st.ID]}
	}
	return out, nil
}

// GetStore loads a store of the org on ctx with the agents that use it.
func (s *Service) GetStore(ctx context.Context, id uuid.UUID) (StoreView, error) {
	store, err := s.stores.StoreByID(ctx, id)
	if err != nil {
		return StoreView{}, s.errOrReport(ctx, "memory: get store", err)
	}
	used, err := s.usedBy(ctx, store.ProjectID)
	if err != nil {
		return StoreView{}, err
	}
	return StoreView{Store: store, UsedBy: used[store.ID]}, nil
}

func (s *Service) usedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]UsedBy, error) {
	if s.bindings == nil {
		return nil, nil
	}
	used, err := s.bindings.StoresUsedBy(ctx, projectID)
	if err != nil {
		return nil, s.unexpected(ctx, "memory: stores used by", err)
	}
	return used, nil
}

// DeleteStore removes a store no published agent version uses, with every fact its driver keeps.
func (s *Service) DeleteStore(ctx context.Context, id uuid.UUID) error {
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		store, err := s.stores.LockStore(ctx, id)
		if err != nil {
			return err
		}
		if s.bindings != nil {
			inUse, err := s.bindings.StoreInUse(ctx, id)
			if err != nil {
				return err
			}
			if inUse {
				return &StoreInUseError{}
			}
		}
		// SECURITY: a store's facts are erased through its own driver first, so no backend keeps facts of a store that is gone.
		facts, err := s.driverFor(store)
		if err != nil {
			return err
		}
		if _, err := facts.ClearStore(ctx, Scope{OrgID: store.OrgID, ProjectID: store.ProjectID, StoreID: store.ID}); err != nil {
			return err
		}
		return s.stores.DeleteStore(ctx, id)
	})
	return s.errOrReport(ctx, "memory: delete store", err)
}
