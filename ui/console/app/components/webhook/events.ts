export interface WebhookEvent {
  name: string
  labelKey: string
}

// Keep in sync with the Go events catalog (internal/kernel/events).
export const webhookEvents: WebhookEvent[] = [
  { name: 'agent.created', labelKey: 'webhooks.event_agent_created' },
  { name: 'agent.updated', labelKey: 'webhooks.event_agent_updated' },
]
