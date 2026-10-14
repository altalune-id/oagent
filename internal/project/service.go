package project

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	slugpkg "altalune.id/oagent/slug"
)

// Service is the project application service: the driving port surfaces call.
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
	OrgID     uuid.UUID
	Slug      string
	Name      string
	CreatedBy uuid.UUID
}

// Create validates and persists a new project within an org. An empty slug is generated, retrying on collision.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Project, error) {
	if req.OrgID == uuid.Nil {
		return nil, apperror.Invalid("organization_id", apperror.FieldRequired)
	}
	if strings.TrimSpace(req.Slug) == "" {
		return s.createGenerated(ctx, req)
	}
	p, err := NewProject(req.OrgID, req.Slug, req.Name, req.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, p); err != nil {
		return nil, s.errOrReport(ctx, "project: create", err)
	}
	return p, nil
}

func (s *Service) createGenerated(ctx context.Context, req CreateRequest) (*Project, error) {
	p, err := slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, func(candidate string) (*Project, error) {
		p, err := NewProject(req.OrgID, candidate, req.Name, req.CreatedBy)
		if err != nil {
			return nil, err
		}
		return p, s.store.Save(ctx, p)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "project: create", err)
	}
	return p, nil
}

// Get loads a project by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Project, error) {
	p, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "project: get", err)
	}
	return p, nil
}

// ByPublicID loads a project by its public id.
func (s *Service) ByPublicID(ctx context.Context, publicID string) (*Project, error) {
	p, err := s.store.ByPublicID(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "project: by public id", err)
	}
	return p, nil
}

// BySlug loads a project by slug within an org.
func (s *Service) BySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error) {
	p, err := s.store.BySlug(ctx, orgID, slug)
	if err != nil {
		return nil, s.errOrReport(ctx, "project: by slug", err)
	}
	return p, nil
}

// List returns an organization's projects.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]*Project, error) {
	projects, err := s.store.List(ctx, orgID)
	if err != nil {
		return nil, s.errOrReport(ctx, "project: list", err)
	}
	return projects, nil
}

// Update changes a project's display name and slug.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name, slug string) (*Project, error) {
	p, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "project: update", err)
	}
	if err := p.Update(name, slug); err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, p); err != nil {
		return nil, s.errOrReport(ctx, "project: update", err)
	}
	return p, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
