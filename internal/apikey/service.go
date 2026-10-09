package apikey

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
)

// Projects resolves an organization's project ids so a grant can only name its own projects.
type Projects interface {
	ProjectIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error)
}

// Service is the API key application service: the driving port surfaces call.
type Service struct {
	store      Store
	scheme     Scheme
	projects   Projects
	unexpected apperror.UnexpectedFunc
	now        func() time.Time
}

// NewService builds a Service over store.
func NewService(store Store, scheme Scheme, projects Projects, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{
		store:      store,
		scheme:     scheme,
		projects:   projects,
		unexpected: unexpected,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	Kind        Kind
	Name        string
	Scopes      []string
	ProjectIDs  []uuid.UUID
	AllProjects bool
	ResourceIDs []uuid.UUID
	ExpiresAt   *time.Time
	CreatedBy   uuid.UUID
}

// Create mints a key and returns it with its one-time plaintext secret.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*APIKey, string, error) {
	if req.OrgID == uuid.Nil {
		return nil, "", apperror.New(apperror.CodeValidation, "organization id is required")
	}
	now := s.now()
	var (
		k         *APIKey
		plaintext string
		err       error
	)
	switch req.Kind {
	case KindProject:
		if req.ProjectID == uuid.Nil {
			return nil, "", apperror.New(apperror.CodeValidation, "project id is required for a project key")
		}
		k, plaintext, err = s.scheme.MintProject(req.OrgID, req.ProjectID, req.Name, req.Scopes, req.ResourceIDs, req.ExpiresAt, now)
	case KindOrg:
		grant := ProjectGrant{All: req.AllProjects, ProjectIDs: req.ProjectIDs}
		if verr := s.requireOrgProjects(ctx, req.OrgID, grant); verr != nil {
			return nil, "", verr
		}
		k, plaintext, err = s.scheme.MintOrg(req.OrgID, req.Name, req.Scopes, grant, req.ExpiresAt, now)
	default:
		return nil, "", apperror.New(apperror.CodeValidation, "invalid key kind: "+string(req.Kind))
	}
	if err != nil {
		return nil, "", err
	}
	k.CreatedBy = req.CreatedBy
	if err := s.store.Save(ctx, k); err != nil {
		return nil, "", s.errOrReport(ctx, "apikey: create", err)
	}
	return k, plaintext, nil
}

// requireOrgProjects rejects a grant naming projects outside the org.
func (s *Service) requireOrgProjects(ctx context.Context, orgID uuid.UUID, grant ProjectGrant) error {
	if grant.All || s.projects == nil {
		return nil
	}
	ids, err := s.projects.ProjectIDs(ctx, orgID)
	if err != nil {
		return s.errOrReport(ctx, "apikey: grant", err)
	}
	allowed := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	for _, id := range grant.ProjectIDs {
		if _, ok := allowed[id]; !ok {
			return apperror.New(apperror.CodeAPIKeyInvalidGrant, "project is not in this organization")
		}
	}
	return nil
}

// List returns an organization's keys.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]*APIKey, error) {
	keys, err := s.store.List(ctx, orgID)
	if err != nil {
		return nil, s.errOrReport(ctx, "apikey: list", err)
	}
	return keys, nil
}

// Revoke marks a key revoked.
func (s *Service) Revoke(ctx context.Context, orgID, id uuid.UUID) error {
	k, err := s.store.ByID(ctx, id)
	if err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	if k.OrgID != orgID {
		return &NotFoundError{ID: id.String()}
	}
	k.Revoke(s.now())
	if err := s.store.Save(ctx, k); err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
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
