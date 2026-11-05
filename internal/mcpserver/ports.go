package mcpserver

import (
	"context"

	"github.com/google/uuid"
)

// Usage reports whether published agent versions grant tools of a server.
type Usage interface {
	ServerInUse(ctx context.Context, serverID uuid.UUID) (bool, error)
}

// Refs turns org and project public ids into ids and back for the handler.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
}
