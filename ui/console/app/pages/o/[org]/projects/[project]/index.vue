<script setup lang="ts">
import CopyButton from '~/components/ui/CopyButton.vue';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

await agents.ensureLoaded(project.value.id);

const items = computed(() => (project.value ? agents.forProject(project.value.id) : []));

useHead({ title: () => `${project.value?.name ?? t('project.title')} · oagent` });
</script>

<template>
  <div v-if="project" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">
        {{ project.name }}
      </h1>
      <p class="text-sm text-base-content/60">
        {{ t('nav.overview') }}
      </p>
      <div class="mt-1 flex items-center gap-1 text-xs text-base-content/60">
        <span class="font-mono">{{ project.id }}</span>
        <CopyButton :value="project.id" :label="t('common.copy')" />
      </div>
    </div>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">
        {{ t('nav.agents') }}
      </h2>
      <div v-if="items.length" class="grid gap-3 sm:grid-cols-2">
        <NuxtLink
          v-for="agent in items"
          :key="agent.id"
          :to="`/o/${org?.slug}/projects/${project.slug}/agents/${agent.slug}`"
          class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
        >
          <div class="card-body">
            <h3 class="card-title text-base">
              {{ agent.name }}
            </h3>
            <p class="text-sm text-base-content/60">
              {{ agent.slug }}
            </p>
            <p v-if="agent.description" class="text-sm text-base-content/50">
              {{ agent.description }}
            </p>
          </div>
        </NuxtLink>
      </div>
      <p v-else class="text-sm text-base-content/60">
        {{ t('agent.empty') }}
      </p>
    </section>
  </div>
</template>
