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
)

// APIKeyService implements apikey.v1.APIKeyService.
type APIKeyService struct {
	keys *Service
}

// NewControlPlane binds the handler to the api key service.
func NewControlPlane(keys *Service) *APIKeyService {
	return &APIKeyService{keys: keys}
}

var _ apikeyv1connect.APIKeyServiceHandler = (*APIKeyService)(nil)

// CreateAPIKey mints a key and returns its one-time plaintext secret.
func (s *APIKeyService) CreateAPIKey(
	ctx context.Context,
	req *connect.Request[apikeyv1.CreateAPIKeyRequest],
) (*connect.Response[apikeyv1.CreateAPIKeyResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	projectID, err := optionalUUID(req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	projectIDs, err := parseUUIDs(req.Msg.GetProjectIds())
	if err != nil {
		return nil, err
	}
	resourceIDs, err := parseUUIDs(req.Msg.GetResourceIds())
	if err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if ts := req.Msg.GetExpiresAt(); ts != nil {
		t := ts.AsTime()
		expiresAt = &t
	}
	k, plaintext, err := s.keys.Create(ctx, CreateRequest{
		OrgID:       orgID,
		ProjectID:   projectID,
		Kind:        Kind(req.Msg.GetKind()),
		Name:        req.Msg.GetName(),
		Scopes:      req.Msg.GetScopes(),
		ProjectIDs:  projectIDs,
		AllProjects: req.Msg.GetAllProjects(),
		ResourceIDs: resourceIDs,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apikeyv1.CreateAPIKeyResponse{Key: keyToProto(k), Plaintext: plaintext}), nil
}

// ListAPIKeys lists an organization's keys, never carrying a plaintext secret.
func (s *APIKeyService) ListAPIKeys(
	ctx context.Context,
	req *connect.Request[apikeyv1.ListAPIKeysRequest],
) (*connect.Response[apikeyv1.ListAPIKeysResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	keys, err := s.keys.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*apikeyv1.APIKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, keyToProto(k))
	}
	return connect.NewResponse(&apikeyv1.ListAPIKeysResponse{Keys: out}), nil
}

// RevokeAPIKey revokes a key by id.
func (s *APIKeyService) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[apikeyv1.RevokeAPIKeyRequest],
) (*connect.Response[apikeyv1.RevokeAPIKeyResponse], error) {
	orgID, err := uuid.Parse(req.Msg.GetOrgId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid key id").WithCause(err)
	}
	if err := s.keys.Revoke(ctx, orgID, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&apikeyv1.RevokeAPIKeyResponse{}), nil
}

func keyToProto(k *APIKey) *apikeyv1.APIKey {
	out := &apikeyv1.APIKey{
		Id:          k.ID.String(),
		OrgId:       k.OrgID.String(),
		Kind:        string(k.Kind),
		Name:        k.Name,
		Scopes:      k.Scopes,
		AllProjects: k.AllProjects,
		SecretHint:  k.SecretHint,
		CreatedAt:   timestamppb.New(k.CreatedAt),
	}
	if k.ProjectID != uuid.Nil {
		out.ProjectId = k.ProjectID.String()
	}
	for _, id := range k.ProjectIDs {
		out.ProjectIds = append(out.ProjectIds, id.String())
	}
	for _, id := range k.ResourceIDs {
		out.ResourceIds = append(out.ResourceIds, id.String())
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
	return out
}

func optionalUUID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	return id, nil
}

func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, apperror.New(apperror.CodeValidation, "invalid uuid").WithCause(err)
		}
		out = append(out, id)
	}
	return out, nil
}
