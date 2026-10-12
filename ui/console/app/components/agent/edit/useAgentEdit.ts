import { useAgentsStore } from '~/stores/agents'
import type { Agent } from '~/stores/agents'
import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export function useAgentEdit() {
  const open = ref(false)
  const id = ref('')
  const name = ref('')
  const description = ref('')
  const busy = ref(false)
  const error = ref<ShownError | null>(null)

  const agents = useAgentsStore()
  const errorMessage = useErrorMessage()
  const { $agentClient: client } = useNuxtApp()

  function show(agent: Agent) {
    id.value = agent.id
    name.value = agent.name
    description.value = agent.description
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
      const res = await client.updateAgent({ id: id.value, name: value, description: description.value })
      if (res.agent) {
        agents.update({ id: res.agent.id, name: res.agent.name, description: res.agent.description })
      }
      open.value = false
      name.value = ''
      description.value = ''
    }
    catch (e) {
      error.value = errorMessage(e, 'error.update_agent')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, description, busy, error, show, hide, submit }
}
