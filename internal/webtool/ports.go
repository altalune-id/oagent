package webtool

import (
	"context"

	"github.com/google/uuid"
)

// Refs turns a project public id into its id and org for the handler; ok is false for an unknown id.
type Refs interface {
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
}
