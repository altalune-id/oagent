package boot

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/sealer"
)

const keyUsageInterval = 30 * time.Second

// Services is every store and service the composition root wires.
type Services struct {
	OrgStore        org.Store
	Orgs            *org.Service
	ProjectStore    project.Store
	Projects        *project.Service
	AgentStore      agent.Store
	Agents          *agent.Service
	APIKeyStore     apikey.Store
	APIKeys         *apikey.Service
	KeyAuthn        *apikey.Authenticator
	KeyUsage        *apikey.UsageWorker
	Outbox          outbox.Store
	WebhookStore    webhook.Store
	Webhooks        *webhook.Service
	Sealer          sealer.Sealer
	Users           *user.Service
	Members         *member.Service
	Sessions        session.Store
	Auth            *auth.Service
	Install         *install.Service
	InstallToken    install.Token
	InstallRequired *atomic.Bool
	Caps            capabilities.Capabilities
	StateSecret     []byte
}

func buildServices(k kernel.Kernel, cfg config.Config) *Services {
	orgStore := org.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	projectStore := project.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	agentStore := agent.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	apiKeyStore := apikey.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)

	caps := capabilities.From(&cfg)
	projects := project.NewService(projectStore, k.Reporter.Unexpected)
	orgs := org.NewService(orgStore, k.Reporter.Unexpected).
		WithCreation(caps.OrgCreation).
		WithAfterCreate(func(ctx context.Context, o *org.Org) error {
			if _, err := projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: project.DefaultSlug, Name: project.DefaultName}); err != nil {
				_ = k.Reporter.Unexpected(ctx, "org: default project", err)
			}
			return nil
		})

	sl := buildSealer(cfg.Secrets)
	outboxStore := outbox.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	webhookStore := webhook.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	webhooks := webhook.NewService(
		webhookStore, k.Log, k.Reporter.Unexpected, sl, outboxStore,
		refResolver{projects: projects, orgs: orgs}, webhookServiceOpts(cfg)...,
	)

	agents := agent.NewService(agentStore, k.Reporter.Unexpected).WithPublisher(webhooks)
	users := user.NewService(user.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix), k.Reporter.Unexpected)
	members := member.NewService(member.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix), k.Reporter.Unexpected)

	keyScheme := apikey.NewScheme("")
	owners := ownerRoles{members: members}
	apiKeys := apikey.NewService(apiKeyStore, keyScheme, projectIDs{projects: projects}, k.Reporter.Unexpected).
		WithOwnerRoles(owners).
		WithPersonalKeys(caps.PersonalKeys)
	keyUsage := apikey.NewUsageWorker(apiKeyStore, keyUsageInterval, k.Log)
	keyAuthn := apikey.NewAuthenticator(apiKeyStore, keyUsage, keyScheme, owners, k.Reporter.Unexpected)
	sessions := buildSessionStore(k, cfg, sl, secretsLog(k.Log, caps))
	authn := auth.NewService(auth.Deps{
		Users:    users,
		Members:  members,
		Orgs:     orgs,
		Projects: projects,
		Sessions: sessions,
		Basic:    basicCredentials(cfg, caps),
	})
	installer := install.NewService(k.Pool, users, orgs, members, projects)

	return &Services{
		OrgStore:     orgStore,
		Orgs:         orgs,
		ProjectStore: projectStore,
		Projects:     projects,
		AgentStore:   agentStore,
		Agents:       agents,
		APIKeyStore:  apiKeyStore,
		APIKeys:      apiKeys,
		KeyAuthn:     keyAuthn,
		KeyUsage:     keyUsage,
		Outbox:       outboxStore,
		WebhookStore: webhookStore,
		Webhooks:     webhooks,
		Sealer:       sl,
		Users:        users,
		Members:      members,
		Sessions:     sessions,
		Auth:         authn,
		Install:      installer,
		Caps:         caps,
	}
}

// webhookServiceOpts allows http:// endpoint URLs outside production (dev/self-hosted).
func webhookServiceOpts(cfg config.Config) []webhook.ServiceOption {
	if cfg.Env.IsProduction() {
		return nil
	}
	return []webhook.ServiceOption{webhook.WithAllowInsecureURL()}
}

type refResolver struct {
	projects *project.Service
	orgs     *org.Service
}

func (r refResolver) ProjectRef(ctx context.Context, projectID uuid.UUID) (slug, publicID string, err error) {
	pr, err := r.projects.Get(ctx, projectID)
	if err != nil {
		return "", "", err
	}
	return pr.Slug, pr.PublicID, nil
}

func (r refResolver) OrgRef(ctx context.Context, orgID uuid.UUID) (publicID string, err error) {
	o, err := r.orgs.Get(ctx, orgID)
	if err != nil {
		return "", err
	}
	return o.PublicID, nil
}

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

type ownerRoles struct{ members *member.Service }

func (o ownerRoles) Role(ctx context.Context, orgID, userID uuid.UUID) (member.Role, bool, error) {
	m, err := o.members.Find(ctx, orgID, userID)
	if member.IsNotFoundError(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return m.Role, true, nil
}
