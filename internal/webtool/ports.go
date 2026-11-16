package webtool

import (
	"context"

	"github.com/google/uuid"
)

// UsedBy names an agent: its id for reach checks, its public id and its name.
type UsedBy struct {
	AgentID        uuid.UUID
	PublicID, Name string
}

// Users names the agents of a project whose latest published version has web search on.
type Users interface {
	WebSearchUsers(ctx context.Context, projectID uuid.UUID) ([]UsedBy, error)
}

// Refs turns a project public id into its id and org for the handler; ok is false for an unknown id.
type Refs interface {
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
}
