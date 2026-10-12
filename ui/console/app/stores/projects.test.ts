import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals'
import { useProjectsStore } from './projects'

const web = { id: 'p1', publicId: 'prj_1', orgId: 'o1', slug: 'web', name: 'Web' }
const api = { id: 'p2', publicId: 'prj_2', orgId: 'o2', slug: 'api', name: 'API' }

describe('useProjectsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('loads the projects of an org', async () => {
    const listProjects = vi.fn().mockResolvedValue({ projects: [web, api] })
    setClient('$projectClient', { listProjects })
    const projects = useProjectsStore()
    await projects.load('o1')
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'o1' })
    expect(projects.items).toEqual([web, api])
    expect(projects.forOrg('o1')).toEqual([web])
    expect(projects.loading).toBe(false)
  })

  it('treats a missing list as empty', async () => {
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({}) })
    const projects = useProjectsStore()
    await projects.load('o1')
    expect(projects.items).toEqual([])
  })

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900')
    setClient('$projectClient', { listProjects: vi.fn().mockRejectedValue(connectError('GEN900')) })
    const projects = useProjectsStore()
    await projects.load('o1')
    expect(projects.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' })
  })

  it('shows the fallback for a plain error and loads again next time', async () => {
    const listProjects = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ projects: [web] })
    setClient('$projectClient', { listProjects })
    const projects = useProjectsStore()
    await projects.ensureLoaded('o1')
    expect(projects.error).toEqual({ message: 'error.load_projects' })
    await projects.ensureLoaded('o1')
    expect(projects.error).toBeNull()
    expect(projects.items).toEqual([web])
  })

  it('shares an in-flight load and skips an org already loaded or empty', async () => {
    const pending = deferred<{ projects: typeof web[] }>()
    const listProjects = vi.fn(() => pending.promise)
    setClient('$projectClient', { listProjects })
    const projects = useProjectsStore()
    await projects.ensureLoaded('')
    const first = projects.ensureLoaded('o1')
    const second = projects.ensureLoaded('o1')
    expect(listProjects).toHaveBeenCalledTimes(1)
    pending.resolve({ projects: [web] })
    await Promise.all([first, second])
    await projects.ensureLoaded('o1')
    expect(listProjects).toHaveBeenCalledTimes(1)
    listProjects.mockResolvedValueOnce({ projects: [api] })
    await projects.ensureLoaded('o2')
    expect(listProjects).toHaveBeenCalledTimes(2)
    expect(projects.items).toEqual([api])
  })

  it('adds a project once per org and slug, and updates it', () => {
    setClient('$projectClient', {})
    const projects = useProjectsStore()
    projects.add(web)
    projects.add({ ...web, id: 'dup' })
    projects.add({ ...web, id: 'p3', orgId: 'o2' })
    expect(projects.items.map(p => p.id)).toEqual(['p1', 'p3'])
    projects.update('p1', { name: 'Website', slug: 'site' })
    expect(projects.items[0]).toEqual({ ...web, name: 'Website', slug: 'site' })
    expect(projects.items[1]?.name).toBe('Web')
  })
})
