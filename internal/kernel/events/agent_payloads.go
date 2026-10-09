package events

import "time"

// AgentSnapshotV1 is the agent shape shared by the agent event payloads; the id is the public (nanoid) id.
type AgentSnapshotV1 struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentCreatedV1 is the payload for AgentCreated.
type AgentCreatedV1 AgentSnapshotV1

// AgentUpdatedV1 is the payload for AgentUpdated.
type AgentUpdatedV1 AgentSnapshotV1
