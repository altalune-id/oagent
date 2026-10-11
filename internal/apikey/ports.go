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
