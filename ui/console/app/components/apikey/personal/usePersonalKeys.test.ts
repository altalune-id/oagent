import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { APIKeySchema } from '../../../../gen/apikey/v1/apikey_pb'
import { connectError, runtimeConfig, setClient, setKnownKeys } from '../../../../test/nuxt-globals'
import { useAPIKeysStore } from '~/stores/apiKeys'
import { useAuthStore } from '~/stores/auth'
import { mcpSnippet } from './mcpSnippet'
import { usePersonalKeys } from './usePersonalKeys'

const now = new Date('2026-10-01T00:00:00Z')
const personalPB = create(APIKeySchema, { id: 'k1', kind: 'personal', name: 'Mine', scopes: ['agent:read', 'mystery:do'] })
const orgPB = create(APIKeySchema, { id: 'k2', kind: 'org', name: 'CI', scopes: ['agent:read'] })
const form = { name: 'Laptop', scopes: ['agent:read'], expiresInHours: 24, allProjects: true, projectIds: ['p1'] }

describe('usePersonalKeys', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>
  let whoami: ReturnType<typeof vi.fn>
  let listProjects: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.setSystemTime(now)
    vi.stubGlobal('window', { location: { origin: 'https://console.test' } })
    client = {
      listAPIKeys: vi.fn().mockResolvedValue({ keys: [personalPB, orgPB] }),
      createAPIKey: vi.fn().mockResolvedValue({ key: create(APIKeySchema, { id: 'k3', kind: 'personal', name: 'Laptop' }), plaintext: 'oak_new' }),
      revokeAPIKey: vi.fn().mockResolvedValue({}),
    }
    whoami = vi.fn().mockResolvedValue({ source: 'local', permissions: ['agent:*', 'member:write'] })
    listProjects = vi.fn().mockResolvedValue({ projects: [{ id: 'p1', publicId: 'prj_1', orgId: 'o1', slug: 'web', name: 'Web' }, { id: 'p2', orgId: 'o2', slug: 'x', name: 'X' }] })
    setClient('$apiKeyClient', client)
    setClient('$authClient', { whoami })
    setClient('$projectClient', { listProjects })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads the viewer, the keys and the projects', async () => {
    const keys = usePersonalKeys('o1')
    expect(keys.signedIn.value).toBe(false)
    expect(keys.mintable.value).toEqual([])
    await keys.load()
    expect(client.listAPIKeys).toHaveBeenCalledWith({ orgId: 'o1' })
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'o1' })
    expect(keys.signedIn.value).toBe(true)
    expect(keys.mine.value.map(k => [k.id, k.scopes])).toEqual([['k1', ['scope.agent_read', 'mystery:do']]])
    expect(keys.mintable.value).toEqual([
      { name: 'agent:read', label: 'scope.agent_read' },
      { name: 'agent:write', label: 'scope.agent_write' },
    ])
    expect(keys.orgProjects.value).toEqual([{ id: 'p1', name: 'Web' }])
  })

  it('is not signed in for a non-local source', async () => {
    whoami.mockResolvedValue({ source: 'apikey', permissions: [] })
    const keys = usePersonalKeys('o1')
    await keys.load()
    expect(keys.signedIn.value).toBe(false)
  })

  it('creates a key for all projects and builds the MCP snippet', async () => {
    runtimeConfig.public.apiUrl = '/base/api'
    const keys = usePersonalKeys('o1')
    await keys.create(form)
    const req = client.createAPIKey!.mock.calls[0]![0]
    expect(req).toMatchObject({ orgId: 'o1', kind: 'personal', name: 'Laptop', scopes: ['agent:read'], allProjects: true, projectIds: [] })
    expect(timestampDate(req.expiresAt)).toEqual(new Date(now.getTime() + 24 * 3600_000))
    expect(useAPIKeysStore().items.map(k => k.id)).toEqual(['k3'])
    expect(keys.created.value).toEqual({ plaintext: 'oak_new', snippet: mcpSnippet('oagent', 'https://console.test/base/mcp', 'oak_new') })
    expect(keys.busy.value).toBe(false)
    keys.dismiss()
    expect(keys.created.value).toBeNull()
  })

  it('sends chosen projects and skips the store when no key comes back', async () => {
    client.createAPIKey!.mockResolvedValue({ plaintext: 'oak_x' })
    const keys = usePersonalKeys('o1')
    await keys.create({ ...form, allProjects: false, projectIds: ['p1', 'p2'] })
    expect(client.createAPIKey!.mock.calls[0]![0]).toMatchObject({ allProjects: false, projectIds: ['p1', 'p2'] })
    expect(useAPIKeysStore().items).toEqual([])
    expect(keys.created.value?.plaintext).toBe('oak_x')
  })

  it('shows create errors', async () => {
    setKnownKeys('apperror.GEN007')
    client.createAPIKey!.mockRejectedValueOnce(connectError('GEN007', { permission: 'agent:write' })).mockRejectedValueOnce(new Error('boom'))
    const keys = usePersonalKeys('o1')
    await keys.create(form)
    expect(keys.error.value).toEqual({ message: 'apperror.GEN007|{"permission":"scope.agent_write"}', reference: 'error.ref|{"id":"req-1"}' })
    expect(keys.created.value).toBeNull()
    await keys.create(form)
    expect(keys.error.value).toEqual({ message: 'error.create_apikey' })
    expect(keys.busy.value).toBe(false)
  })

  it('revokes a key', async () => {
    const keys = usePersonalKeys('o1')
    await keys.load()
    await keys.revoke('k1')
    expect(client.revokeAPIKey).toHaveBeenCalledWith({ orgId: 'o1', id: 'k1' })
    expect(useAPIKeysStore().items.find(k => k.id === 'k1')?.revokedAt).toEqual(now)
    expect(keys.error.value).toBeNull()
  })

  it('shows revoke errors and keeps the key active', async () => {
    setKnownKeys('apperror.GEN900')
    client.revokeAPIKey!.mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'))
    const keys = usePersonalKeys('o1')
    await keys.load()
    await keys.revoke('k1')
    expect(keys.error.value).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' })
    await keys.revoke('k1')
    expect(keys.error.value).toEqual({ message: 'error.revoke_apikey' })
    expect(useAPIKeysStore().items.find(k => k.id === 'k1')?.revokedAt).toBeUndefined()
    expect(useAuthStore().me).not.toBeNull()
  })
})
