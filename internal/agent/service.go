package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	slugpkg "altalune.id/oagent/slug"
)

var errNoLocker = errors.New("agent: publish needs a locker for its references")

// Service is the agent application service: the driving port surfaces call.
type Service struct {
	store      Store
	uow        db.Transactor
	unexpected apperror.UnexpectedFunc
	publisher  Publisher
	projects   tenant.ProjectResolver
	providers  Providers
	servers    MCPServers
	locker     Locker
}

// NewService builds a Service over store; a save and its event run as one unit of work on uow.
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected}
}

// WithPublisher emits agent.created/agent.updated events through p; nil disables emission.
func (s *Service) WithPublisher(p Publisher) *Service {
	s.publisher = p
	return s
}

// WithProjects makes Create refuse a project outside the request's org; nil skips the check.
func (s *Service) WithProjects(r tenant.ProjectResolver) *Service {
	s.projects = r
	return s
}

// WithProviders checks a config's provider and models against p; without it every provider is unknown.
func (s *Service) WithProviders(p Providers) *Service {
	s.providers = p
	return s
}

// WithMCPServers checks a config's MCP server grants against m; without it every server is unknown.
func (s *Service) WithMCPServers(m MCPServers) *Service {
	s.servers = m
	return s
}

// WithLocker share-locks a version's references while it is published; without it every publish fails.
func (s *Service) WithLocker(l Locker) *Service {
	s.locker = l
	return s
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Slug        string
	Name        string
	Description string
	CreatedBy   uuid.UUID
}

// Create validates and persists a new agent within a project. An empty slug is generated, retrying on collision.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Agent, error) {
	if req.OrgID == uuid.Nil {
		return nil, apperror.Invalid("organization_id", apperror.FieldRequired)
	}
	ctx = tenant.WithOrg(ctx, req.OrgID)
	if req.ProjectID == uuid.Nil {
		return nil, apperror.Invalid("project_id", apperror.FieldRequired)
	}
	if err := tenant.RequireProjectInOrg(ctx, s.projects, req.OrgID, req.ProjectID); err != nil {
		if tenant.IsProjectOutsideOrgError(err) {
			return nil, err
		}
		return nil, s.errOrReport(ctx, "agent: project org", err)
	}
	if strings.TrimSpace(req.Slug) == "" {
		return s.createGenerated(ctx, req)
	}
	a, err := New(req.OrgID, req.ProjectID, req.Slug, req.Name, req.Description, req.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.saveAndPublish(ctx, events.AgentCreated, a); err != nil {
		return nil, s.errOrReport(ctx, "agent: create", err)
	}
	return a, nil
}

func (s *Service) createGenerated(ctx context.Context, req CreateRequest) (*Agent, error) {
	a, err := slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, func(candidate string) (*Agent, error) {
		a, err := New(req.OrgID, req.ProjectID, candidate, req.Name, req.Description, req.CreatedBy)
		if err != nil {
			return nil, err
		}
		return a, s.saveAndPublish(ctx, events.AgentCreated, a)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: create", err)
	}
	return a, nil
}

// Get loads an agent by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Agent, error) {
	a, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: get", err)
	}
	return a, nil
}

// Resolve names publicID's agent and its org, in any org; the caller checks reach before it reads the agent.
func (s *Service) Resolve(ctx context.Context, publicID string) (Ref, error) {
	r, err := s.store.Resolve(ctx, publicID)
	if err != nil {
		return Ref{}, s.errOrReport(ctx, "agent: resolve", err)
	}
	return r, nil
}

// List returns a project's agents.
func (s *Service) List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error) {
	agents, err := s.store.List(ctx, projectID)
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: list", err)
	}
	return agents, nil
}

// Update changes an agent's name and description.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name, description string) (*Agent, error) {
	a, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: update", err)
	}
	if err := a.Update(name, description); err != nil {
		return nil, err
	}
	if err := s.saveAndPublish(ctx, events.AgentUpdated, a); err != nil {
		return nil, s.errOrReport(ctx, "agent: update", err)
	}
	return a, nil
}

// Delete removes an agent by id.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return s.errOrReport(ctx, "agent: delete", err)
	}
	return nil
}

// UpdateDraft replaces an agent's draft config; the published version is unchanged.
func (s *Service) UpdateDraft(ctx context.Context, id uuid.UUID, cfg Config) (*Agent, error) {
	// NOTE: last write wins; add a draft revision check once two people edit one agent at a time.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if len(cfg.Tools) > 0 {
		return nil, &InvalidGrantError{Reason: GrantLegacyTools}
	}
	var a *Agent
	err := s.uow.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		if a, err = s.store.LockForPublish(ctx, id); err != nil {
			return err
		}
		if err := s.checkReferences(ctx, a.ProjectID, cfg, false); err != nil {
			return err
		}
		if err := a.SetDraft(cfg, time.Now().UTC()); err != nil {
			return err
		}
		return s.store.SaveDraft(ctx, a)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: update draft", err)
	}
	return a, nil
}

// Publish makes the draft the next version by person by (uuid.Nil for a key); an unchanged draft returns the latest, created false.
func (s *Service) Publish(ctx context.Context, id, by uuid.UUID) (a *Agent, v *Version, created bool, err error) {
	err = s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if a, err = s.store.LockForPublish(ctx, id); err != nil {
			return err
		}
		if a.Draft.Legacy() {
			return &NeedsRepublishError{}
		}
		if s.locker == nil {
			return errNoLocker
		}
		if err := s.locker.LockShared(ctx, []uuid.UUID{a.Draft.ProviderID}, a.Draft.ServerIDs()); err != nil {
			return err
		}
		if err := s.checkReferences(ctx, a.ProjectID, a.Draft, true); err != nil {
			return err
		}
		latest, err := s.store.LatestVersion(ctx, id)
		if err != nil {
			return err
		}
		if v, created, err = a.Publish(latest, by, time.Now().UTC()); err != nil {
			return err
		}
		if created {
			if err := s.store.InsertVersion(ctx, v); err != nil {
				return err
			}
		}
		return s.store.MarkPublished(ctx, a)
	})
	if err != nil {
		return nil, nil, false, s.errOrReport(ctx, "agent: publish", err)
	}
	return a, v, created, nil
}

func (s *Service) checkReferences(ctx context.Context, projectID uuid.UUID, cfg Config, publishing bool) error {
	if cfg.ProviderID != uuid.Nil {
		if err := s.checkModels(ctx, projectID, cfg, publishing); err != nil {
			return err
		}
	}
	for _, g := range cfg.MCPServers {
		if s.servers == nil {
			return &InvalidGrantError{Reason: GrantServer}
		}
		name, err := s.servers.Exists(ctx, projectID, g.ServerID)
		if err != nil {
			return err
		}
		for _, t := range g.Tools {
			if !grantNameRe.MatchString(name + "__" + t.Name) {
				return &InvalidGrantError{Reason: GrantToolName, Name: t.Name}
			}
		}
	}
	return nil
}

func (s *Service) checkModels(ctx context.Context, projectID uuid.UUID, cfg Config, publishing bool) error {
	if s.providers == nil {
		return &InvalidGrantError{Reason: GrantProvider}
	}
	models, err := s.providers.Models(ctx, projectID, cfg.ProviderID)
	if err != nil {
		return err
	}
	priced := make(map[string]bool, len(models))
	for _, m := range models {
		priced[m.ID] = m.Priced
	}
	for _, use := range []struct {
		id     string
		reason GrantReason
	}{{cfg.Model, GrantModel}, {cfg.HelperModel, GrantHelperModel}} {
		if use.id == "" {
			if publishing && use.reason == GrantModel {
				return &InvalidGrantError{Reason: GrantModel}
			}
			continue
		}
		isPriced, ok := priced[use.id]
		if !ok {
			return &InvalidGrantError{Reason: use.reason, Name: use.id}
		}
		if publishing && cfg.MaxCostCents > 0 && !isPriced {
			return &InvalidGrantError{Reason: GrantBudgetUnpriced, Name: use.id}
		}
	}
	return nil
}

// DefaultVersionsListed is how many versions Versions returns when asked for none in particular.
const DefaultVersionsListed = 50

// MaxVersionsListed caps how many versions one Versions call returns.
const MaxVersionsListed = 100

// Versions returns up to limit of an agent's published versions, newest first; limit 0 means DefaultVersionsListed.
func (s *Service) Versions(ctx context.Context, id uuid.UUID, limit int) ([]*Version, error) {
	if limit <= 0 {
		limit = DefaultVersionsListed
	}
	vs, err := s.store.Versions(ctx, id, min(limit, MaxVersionsListed))
	if err != nil {
		return nil, s.errOrReport(ctx, "agent: versions", err)
	}
	return vs, nil
}

// NOTE: one unit of work, so the save and its event commit together or not at all.
func (s *Service) saveAndPublish(ctx context.Context, t events.Type, a *Agent) error {
	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.store.Save(ctx, a); err != nil {
			return err
		}
		return s.publish(ctx, t, a)
	})
}

func (s *Service) publish(ctx context.Context, t events.Type, a *Agent) error {
	if s.publisher == nil {
		return nil
	}
	snapshot := events.AgentSnapshotV1{
		ID: a.PublicID, Slug: a.Slug, Name: a.Name, Description: a.Description, UpdatedAt: a.UpdatedAt,
	}
	var payload any = events.AgentUpdatedV1(snapshot)
	if t == events.AgentCreated {
		payload = events.AgentCreatedV1(snapshot)
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: a.OrgID, ProjectID: a.ProjectID})
	if err := s.publisher.Enqueue(ctx, t, payload); err != nil {
		return fmt.Errorf("agent: publish %s: %w", t, err)
	}
	return nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
