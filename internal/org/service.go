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
	store      Store
	unexpected apperror.UnexpectedFunc
}

// NewService builds a Service over store.
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected}
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	Slug      string
	Name      string
	CreatedBy uuid.UUID
}

// Create validates and persists a new org. An empty slug is generated, retrying on collision.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Org, error) {
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

// List returns every org.
func (s *Service) List(ctx context.Context) ([]*Org, error) {
	orgs, err := s.store.List(ctx)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: list", err)
	}
	return orgs, nil
}

// Rename changes an org's display name.
func (s *Service) Rename(ctx context.Context, id uuid.UUID, name string) (*Org, error) {
	o, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: rename", err)
	}
	if err := o.Rename(name); err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, o); err != nil {
		return nil, s.errOrReport(ctx, "org: rename", err)
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
