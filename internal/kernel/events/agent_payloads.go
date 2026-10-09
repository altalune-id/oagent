package events

import (
	"time"

	"github.com/google/uuid"
)

// AgentSnapshotV1 is the agent shape shared by the agent event payloads.
type AgentSnapshotV1 struct {
	ID          uuid.UUID `json:"id"`
	PublicID    string    `json:"public_id"`
	OrgID       uuid.UUID `json:"org_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentCreatedV1 is the payload for AgentCreated.
type AgentCreatedV1 AgentSnapshotV1

// AgentUpdatedV1 is the payload for AgentUpdated.
type AgentUpdatedV1 AgentSnapshotV1
