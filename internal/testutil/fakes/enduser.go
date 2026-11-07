package fakes

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/enduser"
)

// EndUserStore is an in-memory enduser.Store; Err fails every call.
type EndUserStore struct {
	Err   error
	Users map[uuid.UUID]*enduser.EndUser
	// Identities maps each project's identities to their end user.
	Identities map[uuid.UUID]map[enduser.Identity]uuid.UUID
}

var _ enduser.Store = (*EndUserStore)(nil)

// ByID implements enduser.Store.
func (f *EndUserStore) ByID(_ context.Context, id uuid.UUID) (*enduser.EndUser, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	e, ok := f.Users[id]
	if !ok {
		return nil, &enduser.NotFoundError{ID: id.String()}
	}
	out := *e
	return &out, nil
}

// Owners implements enduser.Store.
func (f *EndUserStore) Owners(_ context.Context, projectID uuid.UUID, ids []enduser.Identity) (map[enduser.Identity]uuid.UUID, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := map[enduser.Identity]uuid.UUID{}
	for _, id := range ids {
		if owner, ok := f.Identities[projectID][id]; ok {
			out[id] = owner
		}
	}
	return out, nil
}

// Save implements enduser.Store.
func (f *EndUserStore) Save(_ context.Context, e *enduser.EndUser) error {
	if f.Err != nil {
		return f.Err
	}
	if f.Users == nil {
		f.Users = map[uuid.UUID]*enduser.EndUser{}
	}
	saved := *e
	f.Users[e.ID] = &saved
	return nil
}

// AddIdentity implements enduser.Store; an identity the project already knows is not added.
func (f *EndUserStore) AddIdentity(_ context.Context, e *enduser.EndUser, id enduser.Identity) (bool, error) {
	if f.Err != nil {
		return false, f.Err
	}
	if f.Identities == nil {
		f.Identities = map[uuid.UUID]map[enduser.Identity]uuid.UUID{}
	}
	if f.Identities[e.ProjectID] == nil {
		f.Identities[e.ProjectID] = map[enduser.Identity]uuid.UUID{}
	}
	if _, ok := f.Identities[e.ProjectID][id]; ok {
		return false, nil
	}
	f.Identities[e.ProjectID][id] = e.ID
	return true, nil
}

// IdentitiesOf implements enduser.Store.
func (f *EndUserStore) IdentitiesOf(_ context.Context, endUserID uuid.UUID) ([]enduser.Identity, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var out []enduser.Identity
	for _, ids := range f.Identities {
		for id, owner := range ids {
			if owner == endUserID {
				out = append(out, id)
			}
		}
	}
	slices.SortFunc(out, func(a, b enduser.Identity) int {
		return strings.Compare(string(a.Kind)+a.Value, string(b.Kind)+b.Value)
	})
	return out, nil
}

// SetDisplayName implements enduser.Store.
func (f *EndUserStore) SetDisplayName(_ context.Context, id uuid.UUID, name string) error {
	if f.Err != nil {
		return f.Err
	}
	e, ok := f.Users[id]
	if !ok {
		return &enduser.NotFoundError{ID: id.String()}
	}
	e.DisplayName = name
	return nil
}

// IdentitiesOfMany implements enduser.Store.
func (f *EndUserStore) IdentitiesOfMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]enduser.Identity, error) {
	out := map[uuid.UUID][]enduser.Identity{}
	for _, id := range ids {
		got, err := f.IdentitiesOf(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = got
	}
	return out, nil
}

// Resolve implements enduser.Store.
func (f *EndUserStore) Resolve(_ context.Context, publicID string) (enduser.Ref, error) {
	if f.Err != nil {
		return enduser.Ref{}, f.Err
	}
	for _, e := range f.Users {
		if e.PublicID == publicID {
			return enduser.Ref{ID: e.ID, OrgID: e.OrgID, ProjectID: e.ProjectID}, nil
		}
	}
	return enduser.Ref{}, &enduser.NotFoundError{ID: publicID}
}

// List implements enduser.Store; it ignores the query and cursor.
func (f *EndUserStore) List(_ context.Context, projectID uuid.UUID, _ string, _ enduser.Cursor, limit int) ([]*enduser.EndUser, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var out []*enduser.EndUser
	for _, e := range f.Users {
		if e.ProjectID == projectID {
			cp := *e
			out = append(out, &cp)
		}
	}
	slices.SortFunc(out, func(a, b *enduser.EndUser) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out[:min(len(out), max(limit, 1))], nil
}
