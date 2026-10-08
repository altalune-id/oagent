import { useProjectClient } from '~/composables/project/useProjectClient'

export interface Project {
  orgId: string
  slug: string
  name: string
}

export const useProjectsStore = defineStore('projects', () => {
  const items = ref<Project[]>([])
  const loading = ref(false)
  const error = ref('')

  const client = useProjectClient()
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

  async function load(orgId: string) {
    loading.value = true
    error.value = ''
    try {
      const res = await client.listProjects({ orgId })
      items.value = (res.projects ?? []).map(p => ({ orgId: p.orgId, slug: p.slug, name: p.name }))
      loadedFor = orgId
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load projects'
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

  return { items, loading, error, forOrg, add, load, ensureLoaded }
})
