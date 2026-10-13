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

// Refs turns org and project public ids into ids; ok is false for an unknown public id.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
}
