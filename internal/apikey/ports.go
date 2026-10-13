package apikey

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/org/member"
)

// Projects resolves an organization's project ids so a grant can only name its own projects.
type Projects interface {
	ProjectIDs(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error)
}

// OwnerRoles finds a person's role in an org; ok is false when they are not a member.
type OwnerRoles interface {
	Role(ctx context.Context, orgID, userID uuid.UUID) (role member.Role, ok bool, err error)
}

// Refs turns org, project and resource public ids into ids and back; ok is false for an unknown public id.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	// ResourceID resolves an agent's or a webhook endpoint's public id within orgID.
	ResourceID(ctx context.Context, orgID uuid.UUID, publicID string) (id uuid.UUID, ok bool, err error)
	// ResourcePublicID reports ok false for a resource that no longer exists.
	ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error)
}
