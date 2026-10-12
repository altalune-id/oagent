import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export interface Project {
  id: string
  publicId: string
  orgId: string
  slug: string
  name: string
}

export const useProjectsStore = defineStore('projects', () => {
  const items = ref<Project[]>([])
  const loading = ref(false)
  const error = ref<ShownError | null>(null)

  const { $projectClient: client } = useNuxtApp()
  const errorMessage = useErrorMessage()
  let loadedFor = ''
  let inflight: Promise<void> | null = null

  function forOrg(orgId: string) {
    return items.value.filter(project => project.orgId === orgId)
  }

  function add(project: Project) {
    if (!items.value.some(p => p.orgId === project.orgId && p.slug === project.slug)) {
      items.value.push(project)
    }
  }

  function update(id: string, patch: { name: string, slug: string }) {
    items.value = items.value.map(p => (p.id === id ? { ...p, name: patch.name, slug: patch.slug } : p))
  }

  async function load(orgId: string) {
    loading.value = true
    error.value = null
    try {
      const res = await client.listProjects({ orgId })
      items.value = (res.projects ?? []).map(p => ({ id: p.id, publicId: p.publicId, orgId: p.orgId, slug: p.slug, name: p.name }))
      loadedFor = orgId
    }
    catch (e) {
      error.value = errorMessage(e, 'error.load_projects')
    }
    finally {
      loading.value = false
    }
  }

  function ensureLoaded(orgId: string) {
    if (!orgId || loadedFor === orgId) {
      return Promise.resolve()
    }
    inflight ??= load(orgId).finally(() => {
      inflight = null
    })
    return inflight
  }

  return { items, loading, error, forOrg, add, update, load, ensureLoaded }
})
