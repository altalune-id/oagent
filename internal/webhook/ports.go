package webhook

import (
	"context"

	"github.com/google/uuid"
)

// Resolver resolves the public identifiers and slug the envelope carries; internal UUIDs are never exposed.
type Resolver interface {
	ProjectRef(ctx context.Context, projectID uuid.UUID) (slug, publicID string, err error)
	OrgRef(ctx context.Context, orgID uuid.UUID) (publicID string, err error)
}
