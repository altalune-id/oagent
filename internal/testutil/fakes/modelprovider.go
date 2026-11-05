package fakes

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/modelprovider"
)

var errLockOutsideTx = errors.New("fakes: a row lock needs a unit of work")

// ModelProviderStore is an in-memory modelprovider.Store; Err fails every call, and locks need a fake unit of work.
type ModelProviderStore struct {
	Err       error
	Providers map[uuid.UUID]*modelprovider.Provider
	// Locks records "update:<id>" and "share:<id>" for each lock taken.
	Locks []string
}

var _ modelprovider.Store = (*ModelProviderStore)(nil)

// Save implements modelprovider.Store; a name taken by another provider in the project is an *AlreadyExistsError.
func (f *ModelProviderStore) Save(_ context.Context, p *modelprovider.Provider) error {
	if f.Err != nil {
		return f.Err
	}
	for _, other := range f.Providers {
		if other.ID != p.ID && other.ProjectID == p.ProjectID && other.Name == p.Name {
			return &modelprovider.AlreadyExistsError{Name: p.Name}
		}
	}
	if f.Providers == nil {
		f.Providers = map[uuid.UUID]*modelprovider.Provider{}
	}
	saved := *p
	f.Providers[p.ID] = &saved
	return nil
}

// Delete implements modelprovider.Store.
func (f *ModelProviderStore) Delete(_ context.Context, id uuid.UUID) error {
	if f.Err != nil {
		return f.Err
	}
	if _, ok := f.Providers[id]; !ok {
		return &modelprovider.NotFoundError{ID: id.String()}
	}
	delete(f.Providers, id)
	return nil
}

// ByID implements modelprovider.Store.
func (f *ModelProviderStore) ByID(_ context.Context, id uuid.UUID) (*modelprovider.Provider, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	p, ok := f.Providers[id]
	if !ok {
		return nil, &modelprovider.NotFoundError{ID: id.String()}
	}
	out := *p
	return &out, nil
}

// List implements modelprovider.Store; it returns the project's providers oldest first.
func (f *ModelProviderStore) List(_ context.Context, projectID uuid.UUID) ([]*modelprovider.Provider, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := []*modelprovider.Provider{}
	for _, p := range f.Providers {
		if p.ProjectID == projectID {
			cp := *p
			out = append(out, &cp)
		}
	}
	slices.SortFunc(out, func(a, b *modelprovider.Provider) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), slices.Compare(a.ID[:], b.ID[:]))
	})
	return out, nil
}

// Resolve implements modelprovider.Store; it names the provider with publicID.
func (f *ModelProviderStore) Resolve(_ context.Context, publicID string) (modelprovider.Ref, error) {
	if f.Err != nil {
		return modelprovider.Ref{}, f.Err
	}
	for _, p := range f.Providers {
		if p.PublicID == publicID {
			return modelprovider.Ref{ID: p.ID, OrgID: p.OrgID, ProjectID: p.ProjectID}, nil
		}
	}
	return modelprovider.Ref{}, &modelprovider.NotFoundError{}
}

// LockForUpdate implements modelprovider.Store like ByID and records the lock; outside a unit of work it fails.
func (f *ModelProviderStore) LockForUpdate(ctx context.Context, id uuid.UUID) (*modelprovider.Provider, error) {
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

// LockShared implements modelprovider.Store; a missing provider is a *NotFoundError, and outside a unit of work it fails.
func (f *ModelProviderStore) LockShared(ctx context.Context, ids []uuid.UUID) error {
	if !InTx(ctx) {
		return errLockOutsideTx
	}
	if f.Err != nil {
		return f.Err
	}
	for _, id := range ids {
		if _, ok := f.Providers[id]; !ok {
			return &modelprovider.NotFoundError{ID: id.String()}
		}
	}
	for _, id := range ids {
		f.Locks = append(f.Locks, "share:"+id.String())
	}
	return nil
}
