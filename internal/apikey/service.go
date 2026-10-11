package apikey

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/scope"
)

// Service is the API key application service: the driving port surfaces call.
type Service struct {
	store        Store
	scheme       Scheme
	projects     Projects
	owners       OwnerRoles
	personalKeys bool
	unexpected   apperror.UnexpectedFunc
	now          func() time.Time
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

// WithOwnerRoles sets the port personal keys check their owner's role against.
func (s *Service) WithOwnerRoles(owners OwnerRoles) *Service {
	s.owners = owners
	return s
}

// WithPersonalKeys turns personal keys on or off; they are off by default.
func (s *Service) WithPersonalKeys(enabled bool) *Service {
	s.personalKeys = enabled
	return s
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
	if req.Kind == KindPersonal {
		if err := s.requirePersonalAllowed(req.CreatedBy); err != nil {
			return nil, "", err
		}
	}
	if req.CreatedBy != uuid.Nil {
		if err := s.requireWithinRole(ctx, req.OrgID, req.CreatedBy, req.Scopes); err != nil {
			return nil, "", err
		}
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
		if verr := s.requireOrgProjects(ctx, req.OrgID, ProjectGrant{ProjectIDs: []uuid.UUID{req.ProjectID}}); verr != nil {
			return nil, "", verr
		}
		k, plaintext, err = s.scheme.MintProject(req.OrgID, req.ProjectID, req.Name, req.Scopes, req.ResourceIDs, req.ExpiresAt, now)
	case KindOrg:
		grant := ProjectGrant{All: req.AllProjects, ProjectIDs: req.ProjectIDs}
		if verr := s.requireOrgProjects(ctx, req.OrgID, grant); verr != nil {
			return nil, "", verr
		}
		k, plaintext, err = s.scheme.MintOrg(req.OrgID, req.Name, req.Scopes, grant, req.ExpiresAt, now)
	case KindPersonal:
		grant := ProjectGrant{All: req.AllProjects, ProjectIDs: req.ProjectIDs}
		if verr := s.requireOrgProjects(ctx, req.OrgID, grant); verr != nil {
			return nil, "", verr
		}
		k, plaintext, err = s.scheme.MintPersonal(req.OrgID, req.CreatedBy, req.Name, req.Scopes, grant, req.ExpiresAt, now)
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

func (s *Service) requirePersonalAllowed(ownerID uuid.UUID) error {
	if !s.personalKeys || s.owners == nil {
		return &PersonalKeysDisabledError{}
	}
	if ownerID == uuid.Nil {
		return apperror.New(apperror.CodeValidation, "a personal key needs an owner")
	}
	return nil
}

// SECURITY: no person can mint a key, of any kind, holding a scope their role does not grant.
func (s *Service) requireWithinRole(ctx context.Context, orgID, userID uuid.UUID, scopes []string) error {
	if s.owners == nil {
		return &ScopeExceedsRoleError{}
	}
	role, ok, err := s.owners.Role(ctx, orgID, userID)
	if err != nil {
		return s.errOrReport(ctx, "apikey: creator role", err)
	}
	if !ok {
		return &ScopeExceedsRoleError{}
	}
	granted := member.BuiltinPermissions(role)
	for _, sc := range scopes {
		if !scope.Grants(granted, sc) {
			return &ScopeExceedsRoleError{Scope: sc}
		}
	}
	return nil
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

// List returns an organization's keys; viewer sees only their own personal keys, uuid.Nil (a trusted caller) sees all.
func (s *Service) List(ctx context.Context, orgID, viewer uuid.UUID) ([]*APIKey, error) {
	keys, err := s.store.List(ctx, orgID)
	if err != nil {
		return nil, s.errOrReport(ctx, "apikey: list", err)
	}
	out := keys[:0:0]
	for _, k := range keys {
		if visibleTo(k, viewer) {
			out = append(out, k)
		}
	}
	return out, nil
}

// Revoke marks a key revoked; another person's personal key is reported as not found.
func (s *Service) Revoke(ctx context.Context, orgID, id, viewer uuid.UUID) error {
	k, err := s.store.ByID(ctx, id)
	if err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	if k.OrgID != orgID || !visibleTo(k, viewer) {
		return &NotFoundError{ID: id.String()}
	}
	k.Revoke(s.now())
	if err := s.store.Save(ctx, k); err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	return nil
}

func visibleTo(k *APIKey, viewer uuid.UUID) bool {
	return viewer == uuid.Nil || k.Kind != KindPersonal || k.CreatedBy == viewer
}

// errOrReport returns err unchanged when it is an expected domain error, else reports it and returns an AppError.
func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
