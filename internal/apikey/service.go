package apikey

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/scope"
)

const mintAttempts = 5

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
		return nil, "", apperror.Invalid("organization_id", apperror.FieldRequired)
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
	mint, err := s.minter(ctx, req)
	if err != nil {
		return nil, "", err
	}
	for range mintAttempts {
		k, plaintext, err := mint(s.now())
		if err != nil {
			return nil, "", err
		}
		k.CreatedBy = req.CreatedBy
		err = s.store.Save(ctx, k)
		if IsHintTakenError(err) {
			continue
		}
		if err != nil {
			return nil, "", s.errOrReport(ctx, "apikey: create", err)
		}
		return k, plaintext, nil
	}
	return nil, "", s.unexpected(ctx, "apikey: create", &HintTakenError{})
}

type mintFunc func(now time.Time) (*APIKey, string, error)

func (s *Service) minter(ctx context.Context, req CreateRequest) (mintFunc, error) {
	switch req.Kind {
	case KindProject:
		if req.ProjectID == uuid.Nil {
			return nil, apperror.Invalid("project_id", apperror.FieldRequired)
		}
		if err := s.requireOrgProjects(ctx, req.OrgID, ProjectGrant{ProjectIDs: []uuid.UUID{req.ProjectID}}); err != nil {
			return nil, err
		}
		return func(now time.Time) (*APIKey, string, error) {
			return s.scheme.MintProject(req.OrgID, req.ProjectID, req.Name, req.Scopes, req.ResourceIDs, req.ExpiresAt, now)
		}, nil
	case KindOrg:
		grant := ProjectGrant{All: req.AllProjects, ProjectIDs: req.ProjectIDs}
		if err := s.requireOrgProjects(ctx, req.OrgID, grant); err != nil {
			return nil, err
		}
		return func(now time.Time) (*APIKey, string, error) {
			return s.scheme.MintOrg(req.OrgID, req.Name, req.Scopes, grant, req.ExpiresAt, now)
		}, nil
	case KindPersonal:
		grant := ProjectGrant{All: req.AllProjects, ProjectIDs: req.ProjectIDs}
		if err := s.requireOrgProjects(ctx, req.OrgID, grant); err != nil {
			return nil, err
		}
		return func(now time.Time) (*APIKey, string, error) {
			return s.scheme.MintPersonal(req.OrgID, req.CreatedBy, req.Name, req.Scopes, grant, req.ExpiresAt, now)
		}, nil
	}
	return nil, apperror.Invalid("kind", apperror.FieldInvalid)
}

func (s *Service) requirePersonalAllowed(ownerID uuid.UUID) error {
	if !s.personalKeys || s.owners == nil {
		return &PersonalKeysDisabledError{}
	}
	if ownerID == uuid.Nil {
		return apperror.Invalid("owner", apperror.FieldRequired)
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
			return &GrantOutsideOrgError{}
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

// Revoke marks a key revoked; another person's personal key is not found, and any key but the viewer's own needs manage.
func (s *Service) Revoke(ctx context.Context, orgID, id, viewer uuid.UUID, manage bool) error {
	k, err := s.store.ByID(ctx, id)
	if err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	return s.revoke(ctx, orgID, k, viewer, manage)
}

func (s *Service) revoke(ctx context.Context, orgID uuid.UUID, k *APIKey, viewer uuid.UUID, manage bool) error {
	if k.OrgID != orgID || !visibleTo(k, viewer) {
		return &NotFoundError{ID: k.ID.String()}
	}
	if !manage && (k.Kind != KindPersonal || k.CreatedBy != viewer) {
		return &authz.ForbiddenError{Permission: scope.ApikeyWrite}
	}
	k.Revoke(s.now())
	if err := s.store.Save(ctx, k); err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	return nil
}

// RevokeByHint revokes orgID's live key whose secret ends with hint (a leading … as shown is ignored), under the same rules as Revoke.
func (s *Service) RevokeByHint(ctx context.Context, orgID uuid.UUID, hint string, viewer uuid.UUID, manage bool) error {
	hint = strings.TrimSpace(hint)
	hint = strings.TrimPrefix(strings.TrimPrefix(hint, "…"), "...")
	k, err := s.store.ByHint(ctx, orgID, hint)
	if err != nil {
		return s.errOrReport(ctx, "apikey: revoke", err)
	}
	return s.revoke(ctx, orgID, k, viewer, manage)
}

// RevokePersonalOf revokes every live personal key userID owns in orgID; it joins the transaction on ctx.
func (s *Service) RevokePersonalOf(ctx context.Context, orgID, userID uuid.UUID) (int, error) {
	n, err := s.store.RevokePersonalOf(ctx, orgID, userID, s.now())
	if err != nil {
		return 0, s.errOrReport(ctx, "apikey: revoke personal keys", err)
	}
	return n, nil
}

func visibleTo(k *APIKey, viewer uuid.UUID) bool {
	return viewer == uuid.Nil || k.Kind != KindPersonal || k.CreatedBy == viewer
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
