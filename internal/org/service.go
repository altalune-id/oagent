package org

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	slugpkg "altalune.id/oagent/slug"
)

const systemSlug = "default"

// Service is the org application service: the driving port surfaces call.
type Service struct {
	store           Store
	uow             db.Transactor
	unexpected      apperror.UnexpectedFunc
	afterCreate     func(ctx context.Context, o *Org) error
	creationAllowed bool
}

// NewService builds a Service over store; Create runs as one unit of work on uow and is allowed until WithCreation(false).
func NewService(store Store, uow db.Transactor, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, uow: uow, unexpected: unexpected, creationAllowed: true}
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

// Create persists a new org and runs the after-create hook in one unit of work; an empty slug is generated, retrying on collision.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Org, error) {
	if !s.creationAllowed {
		return nil, &CreationDisabledError{}
	}
	var hookErr error
	save := func(slug string) (*Org, error) {
		o, err := NewOrg(slug, req.Name, req.CreatedBy)
		if err != nil {
			return nil, err
		}
		return o, s.uow.RunInTx(ctx, func(ctx context.Context) error {
			if err := s.store.Save(ctx, o); err != nil {
				return err
			}
			if s.afterCreate == nil {
				return nil
			}
			hookErr = s.afterCreate(ctx, o)
			return hookErr
		})
	}
	var o *Org
	var err error
	if strings.TrimSpace(req.Slug) == "" {
		o, err = slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, save)
	} else {
		o, err = save(req.Slug)
	}
	switch {
	case hookErr != nil:
		return nil, s.errOrReport(ctx, "org: after create", hookErr)
	case err != nil:
		return nil, s.errOrReport(ctx, "org: create", err)
	}
	return o, nil
}

// CreateSystem persists the system org, skipping the creation gate and after-create hook; an empty slug becomes "default" if free.
func (s *Service) CreateSystem(ctx context.Context, req CreateRequest) (*Org, error) {
	slug, err := s.systemSlug(ctx, req.Slug)
	if err != nil {
		return nil, err
	}
	o, err := NewSystemOrg(slug, req.Name, req.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := s.store.Save(ctx, o); err != nil {
		return nil, s.errOrReport(ctx, "org: create system", err)
	}
	return o, nil
}

// NOTE: checks before inserting, since a unique violation would abort the caller's transaction.
func (s *Service) systemSlug(ctx context.Context, slug string) (string, error) {
	if slug = strings.TrimSpace(slug); slug != "" {
		return slug, nil
	}
	_, err := s.store.BySlug(ctx, systemSlug)
	switch {
	case IsNotFoundError(err):
		return systemSlug, nil
	case err != nil:
		return "", s.errOrReport(ctx, "org: system slug", err)
	}
	return slugpkg.Generate(), nil
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

// ByPublicID loads an org by public id.
func (s *Service) ByPublicID(ctx context.Context, publicID string) (*Org, error) {
	o, err := s.store.ByPublicID(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "org: by public id", err)
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

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
