package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

// PublicIDs is a public id lookup test double: Org, Project, User and Resource register an id and return its new public id.
type PublicIDs struct {
	mu   sync.Mutex
	refs map[string]map[string]publicRef
	pubs map[string]map[uuid.UUID]string
	// Err fails every lookup.
	Err error
}

type publicRef struct{ id, orgID uuid.UUID }

const (
	kindOrg      = "org"
	kindProject  = "project"
	kindUser     = "user"
	kindResource = "resource"
)

// NewPublicIDs returns an empty lookup.
func NewPublicIDs() *PublicIDs {
	return &PublicIDs{refs: map[string]map[string]publicRef{}, pubs: map[string]map[uuid.UUID]string{}}
}

// Org registers an org and returns its public id.
func (f *PublicIDs) Org(id uuid.UUID) string { return f.add(kindOrg, id, uuid.Nil) }

// Project registers a project of orgID and returns its public id.
func (f *PublicIDs) Project(id, orgID uuid.UUID) string { return f.add(kindProject, id, orgID) }

// User registers a user and returns its public id.
func (f *PublicIDs) User(id uuid.UUID) string { return f.add(kindUser, id, uuid.Nil) }

// Resource registers an agent or webhook endpoint of orgID and returns its public id.
func (f *PublicIDs) Resource(id, orgID uuid.UUID) string { return f.add(kindResource, id, orgID) }

func (f *PublicIDs) add(kind string, id, orgID uuid.UUID) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pub, ok := f.pubs[kind][id]; ok {
		return pub
	}
	pub := nanoid.MustNew(21)
	if f.refs[kind] == nil {
		f.refs[kind], f.pubs[kind] = map[string]publicRef{}, map[uuid.UUID]string{}
	}
	f.refs[kind][pub] = publicRef{id: id, orgID: orgID}
	f.pubs[kind][id] = pub
	return pub
}

func (f *PublicIDs) find(kind, publicID string) (publicRef, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return publicRef{}, false, f.Err
	}
	r, ok := f.refs[kind][publicID]
	return r, ok, nil
}

func (f *PublicIDs) publicID(kind string, id uuid.UUID) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	pub, ok := f.pubs[kind][id]
	if !ok {
		return "", fmt.Errorf("fakes: no public id for %s %s", kind, id)
	}
	return pub, nil
}

// OrgID implements the org lookup ports.
func (f *PublicIDs) OrgID(_ context.Context, publicID string) (uuid.UUID, bool, error) {
	r, ok, err := f.find(kindOrg, publicID)
	return r.id, ok, err
}

// IDByPublicID implements invite.OrgIDs; an unknown public id is an error.
func (f *PublicIDs) IDByPublicID(_ context.Context, publicID string) (uuid.UUID, error) {
	r, ok, err := f.find(kindOrg, publicID)
	if err == nil && !ok {
		err = fmt.Errorf("fakes: unknown org %q", publicID)
	}
	return r.id, err
}

// OrgPublicID implements the org lookup ports.
func (f *PublicIDs) OrgPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return f.publicID(kindOrg, id)
}

// ProjectID implements the project lookup ports.
func (f *PublicIDs) ProjectID(_ context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error) {
	r, ok, err := f.find(kindProject, publicID)
	return r.id, r.orgID, ok, err
}

// ProjectPublicID implements the project lookup ports.
func (f *PublicIDs) ProjectPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return f.publicID(kindProject, id)
}

// UserID implements the user lookup ports.
func (f *PublicIDs) UserID(_ context.Context, publicID string) (uuid.UUID, bool, error) {
	r, ok, err := f.find(kindUser, publicID)
	return r.id, ok, err
}

// UserPublicID implements the user lookup ports.
func (f *PublicIDs) UserPublicID(_ context.Context, id uuid.UUID) (string, error) {
	return f.publicID(kindUser, id)
}

// ResourceID implements apikey.Refs; a resource of another org is not found.
func (f *PublicIDs) ResourceID(_ context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, bool, error) {
	r, ok, err := f.find(kindResource, publicID)
	if !ok || r.orgID != orgID {
		return uuid.Nil, false, err
	}
	return r.id, true, err
}

// ResourcePublicID implements apikey.Refs; an unregistered resource is gone (ok false).
func (f *PublicIDs) ResourcePublicID(_ context.Context, _, id uuid.UUID) (publicID string, ok bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", false, f.Err
	}
	publicID, ok = f.pubs[kindResource][id]
	return publicID, ok, nil
}
