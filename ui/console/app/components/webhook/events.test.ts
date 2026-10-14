import { describe, expect, it } from 'vitest';

import { webhookEvents } from './events';

describe('webhookEvents', () => {
  it('lists every subscribable event of the Go catalog', () => {
    expect(webhookEvents.map(e => e.name)).toEqual(['agent.created', 'agent.updated', 'invite.created', 'member.added']);
  });

  it('gives each event a label key built from its name', () => {
    expect(webhookEvents.length).toBeGreaterThan(0);
    for (const event of webhookEvents) {
      expect(event.labelKey).toBe(`webhooks.event_${event.name.replaceAll('.', '_')}`);
    }
  });
});
