package events

// ChannelConnectionV1 is a connection as its events carry it; ids are public ids, with no secret, key, URL or device data.
type ChannelConnectionV1 struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	AgentID   string `json:"agent_id"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
}

// ChannelConnectionCreatedV1 is the payload for ChannelConnectionCreated.
type ChannelConnectionCreatedV1 ChannelConnectionV1

// ChannelConnectionUpdatedV1 is the payload for ChannelConnectionUpdated.
type ChannelConnectionUpdatedV1 ChannelConnectionV1

// ChannelConnectionDeletedV1 is the payload for ChannelConnectionDeleted.
type ChannelConnectionDeletedV1 ChannelConnectionV1
