import { useAgentClient } from '~/composables/agent/useAgentClient'

export interface Agent {
  id: string
  projectId: string
  slug: string
  name: string
  description: string
}

export const useAgentsStore = defineStore('agents', () => {
  const items = ref<Agent[]>([])
  const loading = ref(false)
  const error = ref('')

  const client = useAgentClient()
  let loadedFor = ''
  let inflight: Promise<void> | null = null

  function forProject(projectId: string) {
    return items.value.filter(agent => agent.projectId === projectId)
  }

  function add(agent: Agent) {
    if (!items.value.some(a => a.id === agent.id)) {
      items.value.push(agent)
    }
  }

  function remove(id: string) {
    items.value = items.value.filter(a => a.id !== id)
  }

  async function load(projectId: string) {
    loading.value = true
    error.value = ''
    try {
      const res = await client.listAgents({ projectId })
      items.value = (res.agents ?? []).map(a => ({ id: a.id, projectId: a.projectId, slug: a.slug, name: a.name, description: a.description }))
      loadedFor = projectId
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load agents'
    }
    finally {
      loading.value = false
    }
  }

  function ensureLoaded(projectId: string) {
    if (!projectId || loadedFor === projectId) {
      return Promise.resolve()
    }
    inflight ??= load(projectId).finally(() => {
      inflight = null
    })
    return inflight
  }

  return { items, loading, error, forProject, add, remove, load, ensureLoaded }
})
