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
	kindProvider = "provider"
	kindServer   = "mcpserver"
	kindSource   = "knowledge_source"
	kindStore    = "memory_store"
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

// KnowledgeSource registers a knowledge source of projectID and returns its public id.
func (f *PublicIDs) KnowledgeSource(id, projectID uuid.UUID) string {
	return f.add(kindSource, id, projectID)
}

// MemoryStore registers a memory store of projectID and returns its public id.
func (f *PublicIDs) MemoryStore(id, projectID uuid.UUID) string {
	return f.add(kindStore, id, projectID)
}

// MemoryStoreID implements agent.Refs; it answers the store's project.
func (f *PublicIDs) MemoryStoreID(_ context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	r, ok, err := f.find(kindStore, publicID)
	return r.id, r.orgID, ok, err
}

// MemoryStorePublicID implements agent.Refs; an unregistered store is not ok.
func (f *PublicIDs) MemoryStorePublicID(_ context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	return f.optionalPublicID(kindStore, id)
}

// KnowledgeSourceID implements agent.Refs; it answers the source's project.
func (f *PublicIDs) KnowledgeSourceID(_ context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	r, ok, err := f.find(kindSource, publicID)
	return r.id, r.orgID, ok, err
}

// KnowledgeSourcePublicID implements agent.Refs; an unregistered source is not ok.
func (f *PublicIDs) KnowledgeSourcePublicID(_ context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	return f.optionalPublicID(kindSource, id)
}

// Forget drops id of every kind, as if its row were deleted.
func (f *PublicIDs) Forget(id uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for kind, pubs := range f.pubs {
		if pub, ok := pubs[id]; ok {
			delete(pubs, id)
			delete(f.refs[kind], pub)
		}
	}
}

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

// MembersURL implements invite.OrgIDs with a fixed console path.
func (f *PublicIDs) MembersURL(_ context.Context, _ uuid.UUID) (string, error) {
	return "/console/o/org/settings/members", nil
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

// Provider registers a model provider of projectID and returns its public id.
func (f *PublicIDs) Provider(id, projectID uuid.UUID) string {
	return f.add(kindProvider, id, projectID)
}

// MCPServer registers an MCP server of projectID and returns its public id.
func (f *PublicIDs) MCPServer(id, projectID uuid.UUID) string {
	return f.add(kindServer, id, projectID)
}

// ProviderID implements agent.Refs; it answers the provider's project.
func (f *PublicIDs) ProviderID(_ context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	r, ok, err := f.find(kindProvider, publicID)
	return r.id, r.orgID, ok, err
}

// ProviderPublicID implements agent.Refs; an unregistered provider is not ok.
func (f *PublicIDs) ProviderPublicID(_ context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	return f.optionalPublicID(kindProvider, id)
}

// MCPServerID implements agent.Refs; it answers the server's project.
func (f *PublicIDs) MCPServerID(_ context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	r, ok, err := f.find(kindServer, publicID)
	return r.id, r.orgID, ok, err
}

// MCPServerPublicID implements agent.Refs; an unregistered server is not ok.
func (f *PublicIDs) MCPServerPublicID(_ context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	return f.optionalPublicID(kindServer, id)
}

func (f *PublicIDs) optionalPublicID(kind string, id uuid.UUID) (publicID string, ok bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return "", false, f.Err
	}
	pub, ok := f.pubs[kind][id]
	return pub, ok, nil
}

// Remove forgets id under every kind, as if its row were deleted.
func (f *PublicIDs) Remove(id uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for kind, pubs := range f.pubs {
		if pub, ok := pubs[id]; ok {
			delete(pubs, id)
			delete(f.refs[kind], pub)
		}
	}
}
