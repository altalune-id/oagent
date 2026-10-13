import { useAgentsStore } from '~/stores/agents'
import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export function useAgentCreate(orgId: string, projectId: string) {
  const open = ref(false)
  const name = ref('')
  const slug = ref('')
  const description = ref('')
  const busy = ref(false)
  const error = ref<ShownError | null>(null)

  const agents = useAgentsStore()
  const errorMessage = useErrorMessage()
  const { $agentClient: client } = useNuxtApp()

  function show() {
    error.value = null
    open.value = true
  }

  function hide() {
    if (busy.value) {
      return
    }
    open.value = false
    error.value = null
  }

  async function submit() {
    const value = name.value.trim()
    if (!value || busy.value) {
      return
    }
    busy.value = true
    error.value = null
    try {
      const res = await client.createAgent({ orgId, projectId, slug: slug.value.trim(), name: value, description: description.value })
      const agent = res.agent
      if (agent) {
        agents.add({ id: agent.id, projectId: agent.projectId, slug: agent.slug, name: agent.name, description: agent.description })
        open.value = false
        name.value = ''
        slug.value = ''
        description.value = ''
      }
    }
    catch (e) {
      error.value = errorMessage(e, 'error.create_agent')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, description, busy, error, show, hide, submit }
}
