package boot

import (
	"context"
	"net/url"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
)

type publicIDs struct {
	orgs     *org.Service
	projects *project.Service
	agents   *agent.Service
	users    *user.Service
	webhooks webhook.Store
	console  string

	providers *modelprovider.Service
	servers   *mcpserver.Service
	knowledge *knowledge.Service
	memory    memory.Store
}

func newPublicIDs(s *Services) publicIDs {
	return publicIDs{
		orgs:     s.Orgs,
		projects: s.Projects,
		agents:   s.Agents,
		users:    s.Users,
		webhooks: s.WebhookStore,
		console:  s.ConsoleURL,

		providers: s.ModelProviders,
		servers:   s.MCPServers,
		knowledge: s.Knowledge,
		memory:    s.MemoryStores,
	}
}

// MemoryStoreID resolves a memory store's public id to its id and project.
func (p publicIDs) MemoryStoreID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	r, err := p.memory.ResolveStore(ctx, publicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, notFoundIsAbsent(err)
	}
	return r.ID, r.ProjectID, true, nil
}

// MemoryStorePublicID names a memory store of the org on ctx; a deleted one is not ok.
func (p publicIDs) MemoryStorePublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	s, err := p.memory.StoreByID(ctx, id)
	if err != nil {
		return "", false, notFoundIsAbsent(err)
	}
	return s.PublicID, true, nil
}

// KnowledgeSourceID resolves a knowledge source's public id to its id and project.
func (p publicIDs) KnowledgeSourceID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	if p.knowledge == nil {
		return uuid.Nil, uuid.Nil, false, nil
	}
	r, err := p.knowledge.ResolveSource(ctx, publicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, notFoundIsAbsent(err)
	}
	return r.ID, r.ProjectID, true, nil
}

// KnowledgeSourcePublicID names a knowledge source of the org on ctx; a deleted one is not ok.
func (p publicIDs) KnowledgeSourcePublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	if p.knowledge == nil {
		return "", false, nil
	}
	s, err := p.knowledge.GetSource(ctx, id)
	if err != nil {
		return "", false, notFoundIsAbsent(err)
	}
	return s.PublicID, true, nil
}

func (p publicIDs) OrgID(ctx context.Context, publicID string) (uuid.UUID, bool, error) {
	o, err := p.orgs.ByPublicID(ctx, publicID)
	if err != nil {
		return uuid.Nil, false, notFoundIsAbsent(err)
	}
	return o.ID, true, nil
}

// MembersURL serves invite.OrgIDs.
func (p publicIDs) MembersURL(ctx context.Context, orgID uuid.UUID) (string, error) {
	o, err := p.orgs.Get(ctx, orgID)
	if err != nil {
		return "", err
	}
	return p.console + "/o/" + url.PathEscape(o.Slug) + "/settings/members", nil
}

// IDByPublicID serves invite.OrgIDs; an unknown public id is an *org.NotFoundError.
func (p publicIDs) IDByPublicID(ctx context.Context, publicID string) (uuid.UUID, error) {
	o, err := p.orgs.ByPublicID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	return o.ID, nil
}

func (p publicIDs) OrgPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	o, err := p.orgs.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return o.PublicID, nil
}

func (p publicIDs) ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error) {
	r, err := p.projects.Resolve(ctx, publicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, notFoundIsAbsent(err)
	}
	return r.ID, r.OrgID, true, nil
}

func (p publicIDs) ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	pr, err := p.projects.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return pr.PublicID, nil
}

func (p publicIDs) UserID(ctx context.Context, publicID string) (uuid.UUID, bool, error) {
	u, err := p.users.ByPublicID(ctx, publicID)
	if err != nil {
		return uuid.Nil, false, notFoundIsAbsent(err)
	}
	return u.ID, true, nil
}

func (p publicIDs) UserPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	u, err := p.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	return u.PublicID, nil
}

// ResourceID resolves an agent's or a webhook endpoint's public id within orgID.
func (p publicIDs) ResourceID(ctx context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, bool, error) {
	a, err := p.agents.Resolve(ctx, publicID)
	switch {
	case err == nil && a.OrgID == orgID:
		return a.ID, true, nil
	case err != nil && !isNotFound(err):
		return uuid.Nil, false, err
	}
	e, err := p.webhooks.ByPublicID(tenant.Into(ctx, tenant.Context{OrgID: orgID}), publicID)
	if err != nil {
		return uuid.Nil, false, notFoundIsAbsent(err)
	}
	return e.ID, true, nil
}

func (p publicIDs) ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error) {
	a, err := p.agents.Get(tenant.WithOrg(ctx, orgID), id)
	switch {
	case err == nil && a.OrgID == orgID:
		return a.PublicID, true, nil
	case err != nil && !isNotFound(err):
		return "", false, err
	}
	e, err := p.webhooks.ByID(tenant.Into(ctx, tenant.Context{OrgID: orgID}), id)
	if err != nil {
		return "", false, notFoundIsAbsent(err)
	}
	return e.PublicID, true, nil
}

// ProviderID resolves a model provider's public id to its id and project.
func (p publicIDs) ProviderID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	if p.providers == nil {
		return uuid.Nil, uuid.Nil, false, nil
	}
	r, err := p.providers.Resolve(ctx, publicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, notFoundIsAbsent(err)
	}
	return r.ID, r.ProjectID, true, nil
}

// ProviderPublicID names a model provider of the org on ctx; a deleted one is not ok.
func (p publicIDs) ProviderPublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	if p.providers == nil {
		return "", false, nil
	}
	pr, err := p.providers.Get(ctx, id)
	if err != nil {
		return "", false, notFoundIsAbsent(err)
	}
	return pr.PublicID, true, nil
}

// MCPServerID resolves an MCP server's public id to its id and project.
func (p publicIDs) MCPServerID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error) {
	if p.servers == nil {
		return uuid.Nil, uuid.Nil, false, nil
	}
	r, err := p.servers.Resolve(ctx, publicID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, notFoundIsAbsent(err)
	}
	return r.ID, r.ProjectID, true, nil
}

// MCPServerPublicID names an MCP server of the org on ctx; a deleted one is not ok.
func (p publicIDs) MCPServerPublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error) {
	if p.servers == nil {
		return "", false, nil
	}
	s, err := p.servers.Get(ctx, id)
	if err != nil {
		return "", false, notFoundIsAbsent(err)
	}
	return s.PublicID, true, nil
}

func notFoundIsAbsent(err error) error {
	if isNotFound(err) {
		return nil
	}
	return err
}

func isNotFound(err error) bool {
	ae, ok := apperror.AsAppError(err)
	return ok && apperror.KindOf(ae.Code()) == apperror.KindNotFound
}
