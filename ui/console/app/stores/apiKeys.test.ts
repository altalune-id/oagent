import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { APIKeySchema } from '../../gen/apikey/v1/apikey_pb'
import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals'
import { toAPIKey, useAPIKeysStore } from './apiKeys'
import type { APIKey } from './apiKeys'

const expires = new Date('2027-01-01T00:00:00Z')
const revoked = new Date('2026-06-01T00:00:00Z')
const keyPB = create(APIKeySchema, {
  id: 'k1', kind: 'org', name: 'CI', scopes: ['agent:read'], projectId: '', projectIds: ['p1'], allProjects: false, secretHint: 'abcd',
  expiresAt: timestampFromDate(expires),
})
const key: APIKey = {
  id: 'k1', kind: 'org', name: 'CI', scopes: ['agent:read'], projectId: '', projectIds: ['p1'], allProjects: false, secretHint: 'abcd',
  expiresAt: expires, revokedAt: undefined,
}

describe('toAPIKey', () => {
  it('converts timestamps to dates', () => {
    expect(toAPIKey(keyPB)).toEqual(key)
    const k = toAPIKey(create(APIKeySchema, { id: 'k2', revokedAt: timestampFromDate(revoked) }))
    expect(k.expiresAt).toBeUndefined()
    expect(k.revokedAt).toEqual(revoked)
  })
})

describe('useAPIKeysStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads the keys of an org', async () => {
    const listAPIKeys = vi.fn().mockResolvedValue({ keys: [keyPB] })
    setClient('$apiKeyClient', { listAPIKeys })
    const keys = useAPIKeysStore()
    await keys.load('o1')
    expect(listAPIKeys).toHaveBeenCalledWith({ orgId: 'o1' })
    expect(keys.items).toEqual([key])
    expect(keys.loading).toBe(false)
  })

  it('treats a missing list as empty', async () => {
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockResolvedValue({}) })
    const keys = useAPIKeysStore()
    await keys.load('o1')
    expect(keys.items).toEqual([])
  })

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900')
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockRejectedValue(connectError('GEN900')) })
    const keys = useAPIKeysStore()
    await keys.load('o1')
    expect(keys.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' })
  })

  it('shows the fallback for a plain error', async () => {
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockRejectedValue(new Error('boom')) })
    const keys = useAPIKeysStore()
    await keys.load('o1')
    expect(keys.error).toEqual({ message: 'error.load_apikeys' })
  })

  it('shares an in-flight load and skips an org already loaded or empty', async () => {
    const pending = deferred<{ keys: typeof keyPB[] }>()
    const listAPIKeys = vi.fn(() => pending.promise)
    setClient('$apiKeyClient', { listAPIKeys })
    const keys = useAPIKeysStore()
    await keys.ensureLoaded('')
    const first = keys.ensureLoaded('o1')
    const second = keys.ensureLoaded('o1')
    expect(listAPIKeys).toHaveBeenCalledTimes(1)
    pending.resolve({ keys: [keyPB] })
    await Promise.all([first, second])
    await keys.ensureLoaded('o1')
    expect(listAPIKeys).toHaveBeenCalledTimes(1)
  })

  it('puts a new key first, replacing an older copy, and marks keys revoked', () => {
    vi.useFakeTimers()
    vi.setSystemTime(revoked)
    setClient('$apiKeyClient', {})
    const keys = useAPIKeysStore()
    const other = { ...key, id: 'k2' }
    keys.add(key)
    keys.add(other)
    keys.add({ ...key, name: 'CI 2' })
    expect(keys.items.map(k => [k.id, k.name])).toEqual([['k1', 'CI 2'], ['k2', 'CI']])
    keys.markRevoked('k2')
    expect(keys.items[1]?.revokedAt).toEqual(revoked)
    expect(keys.items[0]?.revokedAt).toBeUndefined()
  })
})
