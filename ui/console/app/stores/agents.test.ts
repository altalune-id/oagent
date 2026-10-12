import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals'
import { useAgentsStore } from './agents'

const bot = { id: 'a1', publicId: 'agt_1', projectId: 'p1', slug: 'bot', name: 'Bot', description: 'Helps' }
const other = { id: 'a2', publicId: 'agt_2', projectId: 'p2', slug: 'other', name: 'Other', description: '' }

describe('useAgentsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('loads the agents of a project', async () => {
    const listAgents = vi.fn().mockResolvedValue({ agents: [bot, other] })
    setClient('$agentClient', { listAgents })
    const agents = useAgentsStore()
    await agents.load('p1')
    expect(listAgents).toHaveBeenCalledWith({ projectId: 'p1' })
    expect(agents.items).toEqual([bot, other])
    expect(agents.forProject('p1')).toEqual([bot])
  })

  it('treats a missing list as empty', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({}) })
    const agents = useAgentsStore()
    await agents.load('p1')
    expect(agents.items).toEqual([])
  })

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900')
    setClient('$agentClient', { listAgents: vi.fn().mockRejectedValue(connectError('GEN900')) })
    const agents = useAgentsStore()
    await agents.load('p1')
    expect(agents.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' })
    expect(agents.loading).toBe(false)
  })

  it('shows the fallback for a plain error', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockRejectedValue(new Error('boom')) })
    const agents = useAgentsStore()
    await agents.load('p1')
    expect(agents.error).toEqual({ message: 'error.load_agents' })
  })

  it('shares an in-flight load and skips a project already loaded or empty', async () => {
    const pending = deferred<{ agents: typeof bot[] }>()
    const listAgents = vi.fn(() => pending.promise)
    setClient('$agentClient', { listAgents })
    const agents = useAgentsStore()
    await agents.ensureLoaded('')
    const first = agents.ensureLoaded('p1')
    const second = agents.ensureLoaded('p1')
    expect(listAgents).toHaveBeenCalledTimes(1)
    pending.resolve({ agents: [bot] })
    await Promise.all([first, second])
    await agents.ensureLoaded('p1')
    expect(listAgents).toHaveBeenCalledTimes(1)
  })

  it('adds, updates and removes agents', () => {
    setClient('$agentClient', {})
    const agents = useAgentsStore()
    agents.add(bot)
    agents.add(bot)
    agents.add(other)
    expect(agents.items).toHaveLength(2)
    agents.update({ id: 'a1', name: 'Robot', description: 'New' })
    expect(agents.items[0]).toEqual({ ...bot, name: 'Robot', description: 'New' })
    expect(agents.items[1]).toEqual(other)
    agents.remove('a1')
    expect(agents.items).toEqual([other])
  })
})
