export interface WebhookEvent {
  name: string;
  labelKey: string;
}

// Keep in sync with the Go events catalog (internal/kernel/events).
export const webhookEvents: WebhookEvent[] = [
  { name: 'agent.created', labelKey: 'webhooks.event_agent_created' },
  { name: 'agent.updated', labelKey: 'webhooks.event_agent_updated' },
  { name: 'invite.created', labelKey: 'webhooks.event_invite_created' },
  { name: 'member.added', labelKey: 'webhooks.event_member_added' },
  { name: 'run.completed', labelKey: 'webhooks.event_run_completed' },
  { name: 'run.failed', labelKey: 'webhooks.event_run_failed' },
  { name: 'channel.connection.created', labelKey: 'webhooks.event_channel_connection_created' },
  { name: 'channel.connection.updated', labelKey: 'webhooks.event_channel_connection_updated' },
  { name: 'channel.connection.deleted', labelKey: 'webhooks.event_channel_connection_deleted' },
];
