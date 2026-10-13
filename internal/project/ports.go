package project

import (
	"context"

	"github.com/google/uuid"
)

// OrgRefs turns an org's public id into its id and back; ok is false for an unknown public id.
type OrgRefs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
}
