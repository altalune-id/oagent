<script setup lang="ts">
import type { Agent } from '~/stores/agents';
import AgentCreateModal from '~/components/agent/create/AgentCreateModal.vue';
import AgentEditModal from '~/components/agent/edit/AgentEditModal.vue';
import CopyButton from '~/components/ui/CopyButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const errorMessage = useErrorMessage();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();
const { $agentClient: client } = useNuxtApp();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

await agents.ensureLoaded(project.value.id);

const items = computed(() => (project.value ? agents.forProject(project.value.id) : []));

const editModal = ref<InstanceType<typeof AgentEditModal>>();
function edit(agent: Agent) {
  editModal.value?.show(agent);
}

async function destroy(id: string) {
  try {
    await client.deleteAgent({ id });
    agents.remove(id);
  } catch (e) {
    agents.error = errorMessage(e, 'error.delete_agent');
  }
}

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('nav.agents')} · oagent` });
</script>

<template>
  <div v-if="project" class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <h1 class="text-2xl font-bold">
        {{ t('nav.agents') }}
      </h1>
      <AgentCreateModal :org-id="org?.id ?? ''" :project-id="project.id" />
    </div>

    <ErrorText v-if="agents.error" :error="agents.error" />
    <div v-else-if="items.length" class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('common.id') }}</th>
            <th>{{ t('common.slug') }}</th>
            <th>{{ t('common.description') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="agent in items" :key="agent.id" class="hover:bg-base-200">
            <td>
              <NuxtLink :to="`/o/${org?.slug}/projects/${project.slug}/agents/${agent.slug}`" class="link">
                {{ agent.name }}
              </NuxtLink>
            </td>
            <td>
              <span class="inline-flex items-center gap-1">
                <span class="font-mono text-xs text-base-content/60">{{ agent.id }}</span>
                <CopyButton :value="agent.id" :label="t('common.copy')" />
              </span>
            </td>
            <td class="text-base-content/60">
              {{ agent.slug }}
            </td>
            <td class="text-base-content/60">
              {{ agent.description }}
            </td>
            <td class="whitespace-nowrap text-right">
              <button class="btn btn-ghost btn-xs" @click="edit(agent)">
                <Icon name="lucide:pencil" size="14" />
                {{ t('common.edit') }}
              </button>
              <button class="btn btn-ghost btn-xs text-error" @click="destroy(agent.id)">
                <Icon name="lucide:trash-2" size="14" />
                {{ t('common.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="text-sm text-base-content/60">
      {{ t('agent.empty') }}
    </p>

    <AgentEditModal ref="editModal" />
  </div>
</template>
