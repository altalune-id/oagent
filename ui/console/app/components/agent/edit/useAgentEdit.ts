import type { ShownError } from '~/lib/apperror/message';

import type { Agent } from '~/stores/agents';
import { create } from '@bufbuild/protobuf';
import { UpdateAgentRequestSchema } from '~~/gen/agent/v1/agent_pb';
import { useValidation } from '~/composables/useValidation';
import { useAgentsStore } from '~/stores/agents';

export function useAgentEdit() {
  const open = ref(false);
  const id = ref('');
  const name = ref('');
  const description = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const agents = useAgentsStore();
  const validation = useValidation(UpdateAgentRequestSchema, {
    fields: ['name', 'description'],
    codes: { AGT004: 'name', AGT005: 'description' },
  });
  const { $agentClient: client } = useNuxtApp();

  function show(agent: Agent) {
    id.value = agent.id;
    name.value = agent.name;
    description.value = agent.description;
    error.value = null;
    validation.clear();
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    open.value = false;
    error.value = null;
    validation.clear();
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { id: id.value, name: name.value.trim(), description: description.value };
    if (!validation.check(create(UpdateAgentRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.updateAgent(req);
      if (res.agent) {
        agents.update({ id: res.agent.id, name: res.agent.name, description: res.agent.description });
      }
      open.value = false;
      name.value = '';
      description.value = '';
    } catch (e) {
      error.value = validation.fromServer(e, 'error.update_agent');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, description, busy, error, fieldErrors: validation.errors, show, hide, submit };
}
