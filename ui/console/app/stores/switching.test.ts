import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals'
import { useAgentsStore } from './agents'
import { useAPIKeysStore } from './apiKeys'
import { useProjectsStore } from './projects'
import { useWebhooksStore } from './webhooks'

interface Case {
  name: string
  client: '$projectClient' | '$agentClient' | '$apiKeyClient' | '$webhookClient'
  method: string
  field: string
  item: (id: string) => Record<string, unknown>
  store: () => { items: { id: string }[], loading: boolean, error: unknown, ensureLoaded: (...ids: string[]) => Promise<void> }
  ids: [string[], string[]]
}

const cases: Case[] = [
  {
    name: 'projects', client: '$projectClient', method: 'listProjects', field: 'projects',
    item: id => ({ id, publicId: id, orgId: 'o', slug: id, name: id }),
    store: () => useProjectsStore(), ids: [['o1'], ['o2']],
  },
  {
    name: 'agents', client: '$agentClient', method: 'listAgents', field: 'agents',
    item: id => ({ id, publicId: id, projectId: 'p', slug: id, name: id, description: '' }),
    store: () => useAgentsStore(), ids: [['p1'], ['p2']],
  },
  {
    name: 'apiKeys', client: '$apiKeyClient', method: 'listAPIKeys', field: 'keys',
    item: id => ({ id, name: id, kind: 1, scopes: [], projectIds: [] }),
    store: () => useAPIKeysStore(), ids: [['o1'], ['o2']],
  },
  {
    name: 'webhooks', client: '$webhookClient', method: 'listWebhooks', field: 'webhooks',
    item: id => ({ id, projectId: 'p', url: 'https://x', eventTypes: [], active: true }),
    store: () => useWebhooksStore(), ids: [['o1', 'p1'], ['o1', 'p2']],
  },
]

describe.each(cases)('$name store when the selection changes mid-load', (c) => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('loads the new selection instead of sharing the old request', async () => {
    const first = deferred<Record<string, unknown>>()
    const second = deferred<Record<string, unknown>>()
    const list = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    setClient(c.client, { [c.method]: list })
    const store = c.store()

    const a = store.ensureLoaded(...c.ids[0])
    const b = store.ensureLoaded(...c.ids[1])
    expect(list).toHaveBeenCalledTimes(2)

    second.resolve({ [c.field]: [c.item('new')] })
    await b
    first.resolve({ [c.field]: [c.item('old')] })
    await a

    expect(store.items.map(i => i.id)).toEqual(['new'])
    expect(store.loading).toBe(false)
  })

  it('ignores a late failure of the old request', async () => {
    setKnownKeys('apperror.GEN900')
    const first = deferred<Record<string, unknown>>()
    const list = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ [c.field]: [c.item('new')] })
    setClient(c.client, { [c.method]: list })
    const store = c.store()

    const a = store.ensureLoaded(...c.ids[0])
    await store.ensureLoaded(...c.ids[1])
    first.reject(connectError('GEN900'))
    await a

    expect(store.error).toBeNull()
    expect(store.items.map(i => i.id)).toEqual(['new'])
  })
})
