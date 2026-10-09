package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	slugpkg "altalune.id/oagent/slug"
)

// Service is the agent application service: the driving port surfaces call.
type Service struct {
	store      Store
	unexpected apperror.UnexpectedFunc
}

// NewService builds a Service over store.
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected}
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
		return nil, apperror.New(apperror.CodeValidation, "organization id is required")
	}
	if req.ProjectID == uuid.Nil {
		return nil, apperror.New(apperror.CodeValidation, "project id is required")
	}
	if strings.TrimSpace(req.Slug) == "" {
		return s.createGenerated(ctx, req)
	}
	a, err := New(req.OrgID, req.ProjectID, req.Slug, req.Name, req.Description, req.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, a); err != nil {
		return nil, s.errOrReport(ctx, "agent: create", err)
	}
	return a, nil
}

// createGenerated mints a default slug, retrying while the slug is already taken in the project.
func (s *Service) createGenerated(ctx context.Context, req CreateRequest) (*Agent, error) {
	a, err := slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, func(candidate string) (*Agent, error) {
		a, err := New(req.OrgID, req.ProjectID, candidate, req.Name, req.Description, req.CreatedBy)
		if err != nil {
			return nil, err
		}
		return a, s.store.Save(ctx, a)
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
	if err := s.store.Save(ctx, a); err != nil {
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

// errOrReport returns err unchanged when it is an expected domain error, else reports it and returns an AppError.
func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
