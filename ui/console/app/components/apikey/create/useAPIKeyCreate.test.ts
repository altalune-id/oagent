import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { APIKeySchema } from '../../../../gen/apikey/v1/apikey_pb'
import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals'
import { useAPIKeysStore } from '~/stores/apiKeys'
import { useAPIKeyCreate } from './useAPIKeyCreate'

const now = new Date('2026-10-01T00:00:00Z')
const keyPB = create(APIKeySchema, { id: 'k1', kind: 'org', name: 'CI' })

describe('useAPIKeyCreate', () => {
  let createAPIKey: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.setSystemTime(now)
    createAPIKey = vi.fn().mockResolvedValue({ key: keyPB, plaintext: 'oak_secret' })
    setClient('$apiKeyClient', { createAPIKey })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function request() {
    return createAPIKey.mock.calls[0]![0]
  }

  it('resets the form when shown', () => {
    const form = useAPIKeyCreate('o1')
    form.name.value = 'old'
    form.kind.value = 'project'
    form.scopes.value = ['agent:read']
    form.allProjects.value = false
    form.projectId.value = 'p1'
    form.projectIds.value = ['p1']
    form.expiresInHours.value = 1
    form.plaintext.value = 'x'
    form.show()
    expect(form.open.value).toBe(true)
    expect([form.name.value, form.kind.value, form.scopes.value, form.allProjects.value, form.projectId.value, form.projectIds.value, form.expiresInHours.value, form.plaintext.value])
      .toEqual(['', 'org', [], true, '', [], 720, ''])
  })

  it('creates an org key for all projects and shows the secret', async () => {
    const form = useAPIKeyCreate('o1')
    form.show()
    form.name.value = ' CI '
    form.scopes.value = ['agent:read']
    form.projectIds.value = ['p9']
    await form.submit()
    const req = request()
    expect(req).toMatchObject({ orgId: 'o1', kind: 'org', name: 'CI', scopes: ['agent:read'], projectId: '', allProjects: true, projectIds: [] })
    expect(timestampDate(req.expiresAt)).toEqual(new Date(now.getTime() + 720 * 3600_000))
    expect(useAPIKeysStore().items.map(k => k.id)).toEqual(['k1'])
    expect(form.plaintext.value).toBe('oak_secret')
    form.hide()
    expect(form.open.value).toBe(false)
    expect(form.plaintext.value).toBe('')
  })

  it('sends chosen projects for an org key limited to some projects', async () => {
    const form = useAPIKeyCreate('o1')
    form.show()
    form.name.value = 'CI'
    form.allProjects.value = false
    form.projectIds.value = ['p1', 'p2']
    form.expiresInHours.value = 1
    await form.submit()
    expect(request()).toMatchObject({ allProjects: false, projectIds: ['p1', 'p2'], projectId: '' })
    expect(timestampDate(request().expiresAt)).toEqual(new Date(now.getTime() + 3600_000))
  })

  it('sends only the project for a project key', async () => {
    createAPIKey.mockResolvedValue({ plaintext: 'oak_p' })
    const form = useAPIKeyCreate('o1')
    form.show()
    form.name.value = 'Deploy'
    form.kind.value = 'project'
    form.allProjects.value = false
    form.projectId.value = 'p1'
    form.projectIds.value = ['p2']
    await form.submit()
    expect(request()).toMatchObject({ kind: 'project', projectId: 'p1', allProjects: false, projectIds: [] })
    expect(useAPIKeysStore().items).toEqual([])
    expect(form.plaintext.value).toBe('oak_p')
  })

  it('does not submit a blank name or while busy', async () => {
    const pending = deferred<object>()
    createAPIKey.mockReturnValue(pending.promise)
    const form = useAPIKeyCreate('o1')
    form.show()
    form.name.value = ' '
    await form.submit()
    expect(createAPIKey).not.toHaveBeenCalled()
    form.name.value = 'CI'
    const first = form.submit()
    await form.submit()
    form.hide()
    expect(form.open.value).toBe(true)
    pending.resolve({ plaintext: '' })
    await first
    expect(createAPIKey).toHaveBeenCalledTimes(1)
  })

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.KEY001')
    createAPIKey.mockRejectedValueOnce(connectError('KEY001')).mockRejectedValueOnce(new Error('boom'))
    const form = useAPIKeyCreate('o1')
    form.show()
    form.name.value = 'CI'
    await form.submit()
    expect(form.error.value).toEqual({ message: 'apperror.KEY001', reference: 'error.ref|{"id":"req-1"}' })
    await form.submit()
    expect(form.error.value).toEqual({ message: 'error.create_apikey' })
    expect(form.plaintext.value).toBe('')
  })
})
