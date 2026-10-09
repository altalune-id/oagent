package boot

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
)

// Services is every store and service the composition root wires.
type Services struct {
	OrgStore     org.Store
	Orgs         *org.Service
	ProjectStore project.Store
	Projects     *project.Service
	AgentStore   agent.Store
	Agents       *agent.Service
	APIKeyStore  apikey.Store
	APIKeys      *apikey.Service
}

func buildServices(k kernel.Kernel, cfg config.Config) *Services {
	orgStore := org.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	projectStore := project.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	agentStore := agent.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	apiKeyStore := apikey.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)

	projects := project.NewService(projectStore, k.Reporter.Unexpected)
	agents := agent.NewService(agentStore, k.Reporter.Unexpected)
	orgs := org.NewService(orgStore, k.Reporter.Unexpected).WithAfterCreate(func(ctx context.Context, o *org.Org) error {
		if _, err := projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: project.DefaultSlug, Name: project.DefaultName}); err != nil {
			_ = k.Reporter.Unexpected(ctx, "org: default project", err)
		}
		return nil
	})
	apiKeys := apikey.NewService(apiKeyStore, apikey.NewScheme(""), projectIDs{projects: projects}, k.Reporter.Unexpected)

	return &Services{
		OrgStore:     orgStore,
		Orgs:         orgs,
		ProjectStore: projectStore,
		Projects:     projects,
		AgentStore:   agentStore,
		Agents:       agents,
		APIKeyStore:  apiKeyStore,
		APIKeys:      apiKeys,
	}
}

// projectIDs adapts the project service to apikey.Projects.
type projectIDs struct{ projects *project.Service }

func (p projectIDs) ProjectIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	list, err := p.projects.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, pr := range list {
		ids = append(ids, pr.ID)
	}
	return ids, nil
}
