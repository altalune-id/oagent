import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { connectError, deferred, navigateTo, setClient, setKnownKeys } from '../../../../test/nuxt-globals'
import { useOrgsStore } from '~/stores/orgs'
import { useUIStore } from '~/stores/ui'
import { useOrgCreate } from './useOrgCreate'

const org = { id: 'V1StGXR8Z5jdHi6BmyT2a', slug: 'acme', name: 'Acme' }

describe('useOrgCreate', () => {
  let createOrg: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    createOrg = vi.fn().mockResolvedValue({ org })
    setClient('$orgClient', { createOrg })
  })

  it('opens through the UI store', () => {
    const form = useOrgCreate()
    form.show()
    expect(form.open.value).toBe(true)
    expect(useUIStore().orgCreateOpen).toBe(true)
  })

  it('creates an org, adds it, closes and goes to it', async () => {
    const form = useOrgCreate()
    form.show()
    form.name.value = ' Acme '
    form.slug.value = ' acme '
    await form.submit()
    expect(createOrg).toHaveBeenCalledWith({ name: 'Acme', slug: 'acme' })
    expect(useOrgsStore().items).toEqual([org])
    expect(form.open.value).toBe(false)
    expect([form.name.value, form.slug.value]).toEqual(['', ''])
    expect(navigateTo).toHaveBeenCalledWith('/o/acme')
  })

  it('stays open when the response has no org', async () => {
    createOrg.mockResolvedValue({})
    const form = useOrgCreate()
    form.show()
    form.name.value = 'Acme'
    await form.submit()
    expect(form.open.value).toBe(true)
    expect(navigateTo).not.toHaveBeenCalled()
  })

  it('does not submit a blank name or while busy', async () => {
    const pending = deferred<{ org: typeof org }>()
    createOrg.mockReturnValue(pending.promise)
    const form = useOrgCreate()
    form.show()
    await form.submit()
    expect(createOrg).not.toHaveBeenCalled()
    form.name.value = 'Acme'
    const first = form.submit()
    await form.submit()
    form.hide()
    expect(form.open.value).toBe(true)
    pending.resolve({ org })
    await first
    expect(createOrg).toHaveBeenCalledTimes(1)
  })

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.ORG002')
    createOrg.mockRejectedValueOnce(connectError('ORG002', { slug: 'acme' })).mockRejectedValueOnce(new Error('boom'))
    const form = useOrgCreate()
    form.show()
    form.name.value = 'Acme'
    await form.submit()
    expect(form.error.value).toEqual({ message: 'apperror.ORG002|{"slug":"acme"}', reference: 'error.ref|{"id":"req-1"}' })
    await form.submit()
    expect(form.error.value).toEqual({ message: 'error.create_org' })
    form.hide()
    expect(form.open.value).toBe(false)
    expect(form.error.value).toBeNull()
  })
})
