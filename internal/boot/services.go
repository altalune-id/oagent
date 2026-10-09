package boot

import (
	"context"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
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
}

func buildServices(k kernel.Kernel, cfg config.Config) *Services {
	orgStore := org.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	projectStore := project.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	agentStore := agent.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)

	projects := project.NewService(projectStore, k.Reporter.Unexpected)
	agents := agent.NewService(agentStore, k.Reporter.Unexpected)
	orgs := org.NewService(orgStore, k.Reporter.Unexpected).WithAfterCreate(func(ctx context.Context, o *org.Org) error {
		if _, err := projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: project.DefaultSlug, Name: project.DefaultName}); err != nil {
			_ = k.Reporter.Unexpected(ctx, "org: default project", err)
		}
		return nil
	})

	return &Services{
		OrgStore:     orgStore,
		Orgs:         orgs,
		ProjectStore: projectStore,
		Projects:     projects,
		AgentStore:   agentStore,
		Agents:       agents,
	}
}
