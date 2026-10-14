<script setup lang="ts">
import CopyButton from '~/components/ui/CopyButton.vue';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const { t } = useI18n();
const route = useRoute();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);
const agent = computed(() =>
  (project.value ? agents.items.find(a => a.projectId === project.value!.id && a.slug === route.params.agent) : undefined),
);

watch(project, (value) => {
  if (value) {
    agents.ensureLoaded(value.id);
  }
}, { immediate: true });

useHead({ title: () => `${agent.value?.name ?? route.params.agent} · ${t('nav.general')} · oagent` });
</script>

<template>
  <div class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">
        {{ t('nav.general') }}
      </h1>
      <div v-if="agent" class="mt-1 flex items-center gap-1 text-xs text-base-content/60">
        <span class="font-mono">{{ agent.id }}</span>
        <CopyButton :value="agent.id" :label="t('common.copy')" />
      </div>
    </div>
    <div class="rounded-box border border-dashed border-base-300 bg-base-100 p-6">
      <p class="text-sm text-base-content/50">
        {{ t('agent.placeholder') }}
      </p>
    </div>
  </div>
</template>
