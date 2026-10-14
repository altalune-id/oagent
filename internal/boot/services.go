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
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/install"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/mailqueue"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/mailer"
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
	KeyScheme       apikey.Scheme
	KeyAuthn        *apikey.Authenticator
	KeyUsage        *apikey.UsageWorker
	Outbox          outbox.Store
	WebhookStore    webhook.Store
	Webhooks        *webhook.Service
	Sealer          sealer.Sealer
	Mailer          mailer.Mailer
	MailQueue       mailqueue.Store
	Users           *user.Service
	Members         *member.Service
	Invites         *invite.Service
	Sessions        session.Store
	Auth            *auth.Service
	Resets          *auth.Resets
	Install         *install.Service
	Legal           *legal.Service
	InstallToken    install.Token
	InstallRequired *atomic.Bool
	Caps            capabilities.Capabilities
	AuthLimiter     ratelimit.Limiter
	StateSecret     []byte
	Validator       *controlplane.Validator
}

func buildServices(k kernel.Kernel, cfg config.Config) *Services {
	orgStore := org.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	projectStore := project.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	agentStore := agent.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	apiKeyStore := apikey.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)

	uow := db.NewTransactor(k.Pool)
	caps := capabilities.From(capabilitiesConfig(cfg))
	projects := project.NewService(projectStore, k.Reporter.Unexpected)
	orgs := org.NewService(orgStore, uow, k.Reporter.Unexpected).
		WithCreation(caps.OrgCreation).
		WithAfterCreate(func(ctx context.Context, o *org.Org) error {
			_, err := projects.Create(ctx, project.CreateRequest{OrgID: o.ID, Slug: project.DefaultSlug, Name: project.DefaultName})
			return err
		})

	sl := buildSealer(cfg.Secrets)
	outboxStore := outbox.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	webhookStore := webhook.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	projectOwners := projectOrgs{projects: projects}
	webhooks := webhook.NewService(
		webhookStore, k.Log, k.Reporter.Unexpected, sl, outboxStore,
		refResolver{projects: projects, orgs: orgs}, append(webhookServiceOpts(cfg), webhook.WithProjects(projectOwners))...,
	)

	agents := agent.NewService(agentStore, uow, k.Reporter.Unexpected).WithPublisher(webhooks).WithProjects(projectOwners)
	users := user.NewService(user.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix), k.Reporter.Unexpected)
	members := member.NewService(member.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix), uow, k.Reporter.Unexpected).
		WithOperatorSupport(caps.OperatorOrgSupport)

	keyScheme := apikey.NewScheme("")
	owners := ownerRoles{members: members}
	apiKeys := apikey.NewService(apiKeyStore, keyScheme, projectIDs{projects: projects}, k.Reporter.Unexpected).
		WithOwnerRoles(owners).
		WithPersonalKeys(caps.PersonalKeys)
	keyUsage := apikey.NewUsageWorker(apiKeyStore, keyUsageInterval, k.Log)
	keyAuthn := apikey.NewAuthenticator(apiKeyStore, keyUsage, keyScheme, owners, k.Reporter.Unexpected)
	sessions := buildSessionStore(k, cfg, sl, secretsLog(k.Log, caps))
	mailQueue := buildMailQueue(k.Pool, cfg, sl)
	authn := auth.NewService(auth.Deps{
		Users:      users,
		Members:    members,
		Orgs:       orgs,
		Projects:   projects,
		Sessions:   sessions,
		Passwords:  users,
		Basic:      basicCredentials(cfg, caps),
		Profiles:   users,
		Transactor: uow,
		Mail:       mailQueue,
		From:       cfg.Mail.From,
		BaseURL:    publicBase(cfg.HTTP),
	})
	installer := install.NewService(uow, users, orgs, members, projects)

	svcs := &Services{
		OrgStore:     orgStore,
		Orgs:         orgs,
		ProjectStore: projectStore,
		Projects:     projects,
		AgentStore:   agentStore,
		Agents:       agents,
		APIKeyStore:  apiKeyStore,
		APIKeys:      apiKeys,
		KeyScheme:    keyScheme,
		KeyAuthn:     keyAuthn,
		KeyUsage:     keyUsage,
		Outbox:       outboxStore,
		WebhookStore: webhookStore,
		Webhooks:     webhooks,
		Sealer:       sl,
		MailQueue:    mailQueue,
		Users:        users,
		Members:      members,
		Sessions:     sessions,
		Auth:         authn,
		Install:      installer,
		Legal:        buildLegal(k, cfg, users),
		Caps:         caps,
		AuthLimiter:  ratelimit.Disabled(),
	}
	svcs.Resets = auth.NewResets(auth.ResetDeps{
		Store:      auth.NewResetStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix),
		Transactor: uow,
		Users:      users,
		Passwords:  users,
		Sessions:   sessions,
		Mail:       svcs.MailQueue,
		From:       cfg.Mail.From,
		BaseURL:    publicBase(cfg.HTTP),
	})
	members.WithEffects(memberEffects(cfg, svcs)).WithDirectory(memberDirectory{users: users, orgs: orgs})
	installer.WithTerms(svcs.Legal)
	svcs.Invites = invite.NewService(invite.Deps{
		Store:      invite.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix),
		Transactor: uow,
		Members:    members,
		Mail:       svcs.MailQueue,
		Users:      users,
		Terms:      svcs.Legal,
		Events:     webhooks,
		Orgs:       memberOrgs{orgs: orgs},
		People:     memberPeople{users: users},
		From:       cfg.Mail.From,
		BaseURL:    publicBase(cfg.HTTP),
		Unexpected: k.Reporter.Unexpected,
	})
	return svcs
}

func buildLegal(k kernel.Kernel, cfg config.Config, users *user.Service) *legal.Service {
	l := legal.NewService(users).WithStore(legal.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix), db.NewTransactor(k.Pool))
	if cfg.Compliance.TermsURL != "" || cfg.Compliance.PrivacyURL != "" {
		l.WithExternalTerms(cfg.Compliance.TermsUpdated())
	}
	return l
}

func memberEffects(cfg config.Config, s *Services) member.Effects {
	return member.Effects{
		Sessions: s.Sessions,
		Keys:     s.APIKeys,
		People:   memberPeople{users: s.Users},
		Orgs:     memberOrgs{orgs: s.Orgs},
		Mail:     s.MailQueue,
		From:     cfg.Mail.From,
		BaseURL:  publicBase(cfg.HTTP),
	}
}

type memberPeople struct{ users *user.Service }

func (p memberPeople) Contact(ctx context.Context, userID uuid.UUID) (email, name string, err error) {
	u, err := p.users.ByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	return u.Email, u.Name, nil
}

func (p memberPeople) PublicID(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := p.users.ByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return u.PublicID, nil
}

type memberDirectory struct {
	users *user.Service
	orgs  *org.Service
}

func (d memberDirectory) UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	u, err := d.users.FindByEmail(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}
	return u.ID, nil
}

func (d memberDirectory) UserPublicID(ctx context.Context, id uuid.UUID) (string, error) {
	u, err := d.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	return u.PublicID, nil
}

func (d memberDirectory) OrgIDByPublicID(ctx context.Context, publicID string) (uuid.UUID, error) {
	o, err := d.orgs.ByPublicID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	return o.ID, nil
}

type memberOrgs struct{ orgs *org.Service }

func (o memberOrgs) Name(ctx context.Context, orgID uuid.UUID) (string, error) {
	got, err := o.orgs.Get(ctx, orgID)
	if err != nil {
		return "", err
	}
	return got.Name, nil
}

func (o memberOrgs) Slug(ctx context.Context, orgID uuid.UUID) (string, error) {
	got, err := o.orgs.Get(ctx, orgID)
	if err != nil {
		return "", err
	}
	return got.Slug, nil
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

type projectOrgs struct{ projects *project.Service }

func (p projectOrgs) OrgOf(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	pr, err := p.projects.Get(ctx, projectID)
	if err != nil {
		return uuid.Nil, err
	}
	return pr.OrgID, nil
}

func capabilitiesConfig(cfg config.Config) capabilities.Config {
	return capabilities.Config{
		Selfhosted: cfg.Mode == config.ModeSelfhosted,
		Cloud:      cfg.Mode == config.ModeCloud,
		BasicAuth:  cfg.Auth.Mode == config.AuthModeBasic,
		EnvGenesis: cfg.Auth.Genesis.Email != "",
	}
}
