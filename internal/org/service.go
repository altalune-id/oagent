package org

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	slugpkg "altalune.id/oagent/slug"
)

// Service is the org application service: the driving port surfaces call.
type Service struct {
	store           Store
	unexpected      apperror.UnexpectedFunc
	afterCreate     func(ctx context.Context, o *Org) error
	creationAllowed bool
}

// NewService builds a Service over store; org creation is allowed until WithCreation(false).
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected, creationAllowed: true}
}

// WithCreation sets whether Create may add orgs; boot passes the OrgCreation capability.
func (s *Service) WithCreation(allowed bool) *Service {
	s.creationAllowed = allowed
	return s
}

// WithAfterCreate registers a hook run after an org is created (e.g. to seed a default project).
func (s *Service) WithAfterCreate(hook func(ctx context.Context, o *Org) error) *Service {
	s.afterCreate = hook
	return s
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	Slug      string
	Name      string
	CreatedBy uuid.UUID
}

// Create validates and persists a new org. An empty slug is generated, retrying on collision.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Org, error) {
	if !s.creationAllowed {
		return nil, &CreationDisabledError{}
	}
	o, err := s.create(ctx, req)
	if err != nil {
		return nil, err
	}
	if s.afterCreate != nil {
		if herr := s.afterCreate(ctx, o); herr != nil {
			return nil, s.errOrReport(ctx, "org: after create", herr)
		}
	}
	return o, nil
}

// create persists a new org; Create applies the afterCreate hook on success.
func (s *Service) create(ctx context.Context, req CreateRequest) (*Org, error) {
	if strings.TrimSpace(req.Slug) == "" {
		return s.createGenerated(ctx, req)
	}
	o, err := NewOrg(req.Slug, req.Name, req.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, o); err != nil {
		return nil, s.errOrReport(ctx, "org: create", err)
	}
	return o, nil
}

// createGenerated mints a default slug, retrying while the slug is already taken.
func (s *Service) createGenerated(ctx context.Context, req CreateRequest) (*Org, error) {
	o, err := slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, func(candidate string) (*Org, error) {
		o, err := NewOrg(candidate, req.Name, req.CreatedBy)
		if err != nil {
			return nil, err
		}
		return o, s.store.Save(ctx, o)
	})
	if err != nil {
		return nil, s.errOrReport(ctx, "org: create", err)
	}
	return o, nil
}

// Get loads an org by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Org, error) {
	o, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: get", err)
	}
	return o, nil
}

// BySlug loads an org by slug.
func (s *Service) BySlug(ctx context.Context, slug string) (*Org, error) {
	o, err := s.store.BySlug(ctx, slug)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: by slug", err)
	}
	return o, nil
}

// System returns the deployment's system org.
func (s *Service) System(ctx context.Context) (*Org, error) {
	o, err := s.store.System(ctx)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: system", err)
	}
	return o, nil
}

// List returns every org.
func (s *Service) List(ctx context.Context) ([]*Org, error) {
	orgs, err := s.store.List(ctx)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: list", err)
	}
	return orgs, nil
}

// Update changes an org's display name and slug.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name, slug string) (*Org, error) {
	o, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: update", err)
	}
	if err := o.Update(name, slug); err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, o); err != nil {
		return nil, s.errOrReport(ctx, "org: update", err)
	}
	return o, nil
}

// errOrReport returns err unchanged when it is an expected domain error, else reports it and returns an AppError.
func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
