import { describe, expect, it } from 'vitest'

import { webhookEvents } from './events'

describe('webhookEvents', () => {
  it('gives each event a label key built from its name', () => {
    expect(webhookEvents.length).toBeGreaterThan(0)
    for (const event of webhookEvents) {
      expect(event.labelKey).toBe(`webhooks.event_${event.name.replaceAll('.', '_')}`)
    }
  })
})
