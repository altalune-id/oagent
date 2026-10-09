package events

// WebhookPingV1 is the payload for WebhookPing.
type WebhookPingV1 struct {
	EndpointID string `json:"endpoint_id"`
}
