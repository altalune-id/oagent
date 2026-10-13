import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { fakeStorage, setClient } from '../../../test/nuxt-globals'
import { useOrgsStore } from '~/stores/orgs'
import { usePinnedStore } from '~/stores/pinned'
import { useLanding } from './useLanding'

const acme = { id: 'V1StGXR8Z5jdHi6BmyT2a', slug: 'acme', name: 'Acme' }
const beta = { id: 'YeDxg2kMYznPJIG1wVmij', slug: 'beta', name: 'Beta' }
const web = { id: 'x6LH9WPWiktLwCBKHy4oF', orgId: 'V1StGXR8Z5jdHi6BmyT2a', slug: 'web', name: 'Web' }
const api = { id: 'KdrF3t1FHtooBPmLgj4sy', orgId: 'V1StGXR8Z5jdHi6BmyT2a', slug: 'api', name: 'API' }

describe('useLanding', () => {
  let listProjects: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('localStorage', fakeStorage())
    listProjects = vi.fn().mockResolvedValue({ projects: [web, api] })
    setClient('$orgClient', {})
    setClient('$projectClient', { listProjects })
  })

  function withOrgs(...list: typeof acme[]) {
    const orgs = useOrgsStore()
    list.forEach(org => orgs.add(org))
  }

  it('goes to the org list when there are no orgs', async () => {
    expect(await useLanding().resolve()).toBe('/orgs')
    expect(listProjects).not.toHaveBeenCalled()
  })

  it('lands on the first project of the first org', async () => {
    withOrgs(acme, beta)
    expect(await useLanding().resolve()).toBe('/o/acme/projects/web')
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'V1StGXR8Z5jdHi6BmyT2a' })
  })

  it('lands on the pinned org and project', async () => {
    withOrgs(beta, acme)
    usePinnedStore().setProject('acme', 'api')
    expect(await useLanding().resolve()).toBe('/o/acme/projects/api')
  })

  it('uses the requested org, falling back to the first org for an unknown slug', async () => {
    withOrgs(acme, beta)
    listProjects.mockResolvedValue({ projects: [] })
    expect(await useLanding().resolve('beta')).toBe('/o/beta')
    expect(await useLanding().resolve('nope')).toBe('/o/acme')
  })
})
