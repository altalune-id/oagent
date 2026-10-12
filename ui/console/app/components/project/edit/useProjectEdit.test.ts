import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals'
import { useProjectsStore } from '~/stores/projects'
import { useProjectEdit } from './useProjectEdit'

const project = { id: 'p1', publicId: 'prj_1', orgId: 'o1', slug: 'web', name: 'Web' }

describe('useProjectEdit', () => {
  let updateProject: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    updateProject = vi.fn().mockResolvedValue({ project: { ...project, name: 'Site', slug: 'site' } })
    setClient('$projectClient', { updateProject })
    useProjectsStore().add(project)
  })

  it('fills the form, renames the project in the store and closes', async () => {
    const form = useProjectEdit()
    form.show(project)
    expect([form.open.value, form.name.value, form.slug.value]).toEqual([true, 'Web', 'web'])
    form.name.value = ' Site '
    form.slug.value = ' site '
    await form.submit()
    expect(updateProject).toHaveBeenCalledWith({ id: 'p1', name: 'Site', slug: 'site' })
    expect(useProjectsStore().items[0]).toEqual({ ...project, name: 'Site', slug: 'site' })
    expect([form.open.value, form.name.value, form.slug.value]).toEqual([false, '', ''])
  })

  it('closes without touching the store when the response has no project', async () => {
    updateProject.mockResolvedValue({})
    const form = useProjectEdit()
    form.show(project)
    await form.submit()
    expect(useProjectsStore().items[0]).toEqual(project)
    expect(form.open.value).toBe(false)
  })

  it('does not submit a blank name or while busy', async () => {
    const pending = deferred<object>()
    updateProject.mockReturnValue(pending.promise)
    const form = useProjectEdit()
    form.show(project)
    form.name.value = ''
    await form.submit()
    expect(updateProject).not.toHaveBeenCalled()
    form.name.value = 'Site'
    const first = form.submit()
    await form.submit()
    form.hide()
    expect(form.open.value).toBe(true)
    pending.resolve({})
    await first
    expect(updateProject).toHaveBeenCalledTimes(1)
  })

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.PRJ002')
    updateProject.mockRejectedValueOnce(connectError('PRJ002')).mockRejectedValueOnce(new Error('boom'))
    const form = useProjectEdit()
    form.show(project)
    await form.submit()
    expect(form.error.value).toEqual({ message: 'apperror.PRJ002', reference: 'error.ref|{"id":"req-1"}' })
    await form.submit()
    expect(form.error.value).toEqual({ message: 'error.rename_project' })
    form.hide()
    expect(form.open.value).toBe(false)
    expect(form.error.value).toBeNull()
  })
})
