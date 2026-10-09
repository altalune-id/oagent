package events

import "github.com/google/uuid"

// WebhookPingV1 is the payload for WebhookPing.
type WebhookPingV1 struct {
	EndpointID uuid.UUID `json:"endpoint_id"`
}
