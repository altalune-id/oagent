import type { ComputedRef } from 'vue';
import type { Agent } from '~/stores/agents';
import { useProject } from '~/components/project/useProject';
import { sampleAgentSlug } from '~/preview/samples';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useKnowledgeStore } from '~/stores/knowledge';
import { useMcpServersStore } from '~/stores/mcpServers';
import { useModelProvidersStore } from '~/stores/modelProviders';

// useAgentPage resolves the agent in the route with the project data its pages read, and throws a 404 when it is unknown.
export async function useAgentPage() {
  const route = useRoute();
  const { t } = useI18n();
  const { project } = useProject();
  const agents = useAgentsStore();
  const providers = useModelProvidersStore();
  const servers = useMcpServersStore();
  const knowledge = useKnowledgeStore();
  const projectId = project.value!.id;

  await Promise.all([
    agents.ensureLoaded(projectId),
    useAuthStore().load(),
    providers.ensureLoaded(projectId),
    servers.ensureLoaded(projectId),
    knowledge.ensureLoaded(projectId),
  ]);

  const found = computed(() => agents.items.find(a => a.projectId === projectId && a.slug === route.params.agent));
  if (!found.value) {
    throw createError({ statusCode: 404, statusMessage: t('error.agent_not_found'), fatal: true });
  }
  const agent = found as ComputedRef<Agent>;
  const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents/${route.params.agent}`);
  const usesSample = computed(() => agent.value.slug === sampleAgentSlug);

  return { project, agent, providers, servers, knowledge, base, usesSample };
}
