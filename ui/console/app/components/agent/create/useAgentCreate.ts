import type { ShownError } from '~/lib/apperror/message';

import { create } from '@bufbuild/protobuf';
import { CreateAgentRequestSchema } from '~~/gen/agent/v1/agent_pb';
import { useValidation } from '~/composables/useValidation';
import { useAgentsStore } from '~/stores/agents';

export function useAgentCreate(orgId: string, projectId: string) {
  const open = ref(false);
  const name = ref('');
  const slug = ref('');
  const description = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const agents = useAgentsStore();
  const validation = useValidation(CreateAgentRequestSchema, {
    fields: ['name', 'slug', 'description'],
    codes: { AGT002: 'slug', AGT003: 'slug', AGT004: 'name', AGT005: 'description' },
  });
  const { $agentClient: client } = useNuxtApp();

  function show() {
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
    const req = { orgId, projectId, slug: slug.value.trim(), name: name.value.trim(), description: description.value };
    if (!validation.check(create(CreateAgentRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createAgent(req);
      const agent = res.agent;
      if (agent) {
        agents.add({ id: agent.id, projectId: agent.projectId, slug: agent.slug, name: agent.name, description: agent.description });
        open.value = false;
        name.value = '';
        slug.value = '';
        description.value = '';
      }
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_agent');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, slug, description, busy, error, fieldErrors: validation.errors, show, hide, submit };
}
