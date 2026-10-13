package apikey

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/scope"
)

// APIKeyService implements apikey.v1.APIKeyService; ids are public ids and a key is named by its hint.
type APIKeyService struct {
	keys *Service
	refs Refs
}

// NewControlPlane binds the handler to the api key service and the public id lookups.
func NewControlPlane(keys *Service, refs Refs) *APIKeyService {
	return &APIKeyService{keys: keys, refs: refs}
}

var _ apikeyv1connect.APIKeyServiceHandler = (*APIKeyService)(nil)

// CreateAPIKey mints a key and returns its one-time plaintext secret.
func (s *APIKeyService) CreateAPIKey(
	ctx context.Context,
	req *connect.Request[apikeyv1.CreateAPIKeyRequest],
) (*connect.Response[apikeyv1.CreateAPIKeyResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId(), authz.ReachOrg)
	if err != nil {
		return nil, err
	}
	var projectID uuid.UUID
	if raw := req.Msg.GetProjectId(); raw != "" {
		if projectID, err = s.project(ctx, orgID, raw); err != nil {
			return nil, err
		}
	}
	projectIDs := make([]uuid.UUID, 0, len(req.Msg.GetProjectIds()))
	for _, raw := range req.Msg.GetProjectIds() {
		id, err := s.project(ctx, orgID, raw)
		if err != nil {
			return nil, err
		}
		projectIDs = append(projectIDs, id)
	}
	resourceIDs := make([]uuid.UUID, 0, len(req.Msg.GetResourceIds()))
	for _, raw := range req.Msg.GetResourceIds() {
		id, ok, err := s.refs.ResourceID(ctx, orgID, raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperror.Invalid("resource_ids", apperror.FieldInvalid)
		}
		resourceIDs = append(resourceIDs, id)
	}
	var expiresAt *time.Time
	if ts := req.Msg.GetExpiresAt(); ts != nil {
		t := ts.AsTime()
		expiresAt = &t
	}
	kind := Kind(req.Msg.GetKind())
	p := session.PrincipalFrom(ctx)
	switch {
	case kind == KindPersonal && !p.IsPerson():
		return nil, &authn.UnauthorizedError{}
	case kind != KindPersonal && !manages(p):
		return nil, &authz.ForbiddenError{Permission: scope.ApikeyWrite}
	}
	var createdBy uuid.UUID
	if p.IsPerson() {
		createdBy = p.UserID
	}
	k, plaintext, err := s.keys.Create(ctx, CreateRequest{
		OrgID:       orgID,
		ProjectID:   projectID,
		Kind:        kind,
		Name:        req.Msg.GetName(),
		Scopes:      req.Msg.GetScopes(),
		ProjectIDs:  projectIDs,
		AllProjects: req.Msg.GetAllProjects(),
		ResourceIDs: resourceIDs,
		ExpiresAt:   expiresAt,
		CreatedBy:   createdBy,
	})
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, k, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apikeyv1.CreateAPIKeyResponse{Key: out, Plaintext: plaintext}), nil
}

// ListAPIKeys lists an org's keys, without plaintext. SECURITY: keys never see personal keys; project-limited keys see none.
func (s *APIKeyService) ListAPIKeys(
	ctx context.Context,
	req *connect.Request[apikeyv1.ListAPIKeysRequest],
) (*connect.Response[apikeyv1.ListAPIKeysResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId(), authz.ReachWholeOrg)
	if err != nil {
		return nil, err
	}
	p := session.PrincipalFrom(ctx)
	keys, err := s.keys.List(ctx, orgID, p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]*apikeyv1.APIKey, 0, len(keys))
	for _, k := range keys {
		if p.Source == session.SourceAPIKey && k.Kind == KindPersonal {
			continue
		}
		pk, err := s.toProto(ctx, k, req.Msg.GetOrgId())
		if err != nil {
			return nil, err
		}
		out = append(out, pk)
	}
	return connect.NewResponse(&apikeyv1.ListAPIKeysResponse{Keys: out}), nil
}

// RevokeAPIKey revokes the org's live key with the given hint.
func (s *APIKeyService) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[apikeyv1.RevokeAPIKeyRequest],
) (*connect.Response[apikeyv1.RevokeAPIKeyResponse], error) {
	orgID, err := s.org(ctx, req.Msg.GetOrgId(), authz.ReachOrg)
	if err != nil {
		return nil, err
	}
	p := session.PrincipalFrom(ctx)
	manage := manages(p)
	if !manage && !p.IsPerson() {
		return nil, &authz.ForbiddenError{Permission: scope.ApikeyWrite}
	}
	if err := s.keys.RevokeByHint(ctx, orgID, req.Msg.GetHint(), p.UserID, manage); err != nil {
		return nil, err
	}
	return connect.NewResponse(&apikeyv1.RevokeAPIKeyResponse{}), nil
}

// SECURITY: a member manages only their own personal keys; other keys need apikey:write. No Principal (pre-lockdown) manages all.
func manages(p session.Principal) bool {
	return p.Source == "" || authz.Allowed(p, scope.ApikeyWrite)
}

// SECURITY: an unknown org and an org out of reach answer the same not-found.
func (s *APIKeyService) org(ctx context.Context, publicID string, reach func(context.Context, uuid.UUID) bool) (uuid.UUID, error) {
	id, ok, err := s.refs.OrgID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !ok || !reach(ctx, id) {
		return uuid.Nil, &NotFoundError{}
	}
	return id, nil
}

// SECURITY: an unknown project and another org's project are the same refused grant.
func (s *APIKeyService) project(ctx context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, error) {
	id, projectOrg, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return uuid.Nil, err
	}
	if !ok || projectOrg != orgID {
		return uuid.Nil, &GrantOutsideOrgError{}
	}
	return id, nil
}

func (s *APIKeyService) toProto(ctx context.Context, k *APIKey, orgPublicID string) (*apikeyv1.APIKey, error) {
	out := &apikeyv1.APIKey{
		OrgId:       orgPublicID,
		Kind:        string(k.Kind),
		Name:        k.Name,
		Scopes:      k.Scopes,
		AllProjects: k.AllProjects,
		SecretHint:  k.SecretHint,
		CreatedAt:   timestamppb.New(k.CreatedAt),
	}
	var err error
	if k.ProjectID != uuid.Nil {
		if out.ProjectId, err = s.refs.ProjectPublicID(ctx, k.ProjectID); err != nil {
			return nil, err
		}
	}
	for _, id := range k.ProjectIDs {
		pub, err := s.refs.ProjectPublicID(ctx, id)
		if err != nil {
			return nil, err
		}
		out.ProjectIds = append(out.ProjectIds, pub)
	}
	for _, id := range k.ResourceIDs {
		pub, ok, err := s.refs.ResourcePublicID(ctx, k.OrgID, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out.ResourceIds = append(out.ResourceIds, pub)
		}
	}
	if k.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*k.ExpiresAt)
	}
	if k.RevokedAt != nil {
		out.RevokedAt = timestamppb.New(*k.RevokedAt)
	}
	if k.LastUsedAt != nil {
		out.LastUsedAt = timestamppb.New(*k.LastUsedAt)
	}
	return out, nil
}
