import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export interface Agent {
  id: string
  publicId: string
  projectId: string
  slug: string
  name: string
  description: string
}

export const useAgentsStore = defineStore('agents', () => {
  const items = ref<Agent[]>([])
  const loading = ref(false)
  const error = ref<ShownError | null>(null)

  const { $agentClient: client } = useNuxtApp()
  const errorMessage = useErrorMessage()
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

  function update(patch: Pick<Agent, 'id' | 'name' | 'description'>) {
    items.value = items.value.map(a => (a.id === patch.id ? { ...a, name: patch.name, description: patch.description } : a))
  }

  async function load(projectId: string) {
    loading.value = true
    error.value = null
    try {
      const res = await client.listAgents({ projectId })
      items.value = (res.agents ?? []).map(a => ({ id: a.id, publicId: a.publicId, projectId: a.projectId, slug: a.slug, name: a.name, description: a.description }))
      loadedFor = projectId
    }
    catch (e) {
      error.value = errorMessage(e, 'error.load_agents')
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

  return { items, loading, error, forProject, add, remove, update, load, ensureLoaded }
})
