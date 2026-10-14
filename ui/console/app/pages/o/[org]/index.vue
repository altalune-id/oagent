<script setup lang="ts">
import type { StatItem } from '~/components/ui/types';
import AppStats from '~/components/ui/AppStats.vue';
import CopyButton from '~/components/ui/CopyButton.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const items = computed(() => (org.value ? projects.forOrg(org.value.id) : []));

const stats = computed<StatItem[]>(() => [
  { label: t('nav.projects'), value: items.value.length },
  { label: t('stat.agents'), value: 0 },
  { label: t('stat.members'), value: 1 },
]);

useHead({ title: () => `${org.value?.name ?? t('org.title')} · oagent` });
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">
        {{ org.name }}
      </h1>
      <p class="text-sm text-base-content/60">
        {{ t('nav.overview') }}
      </p>
      <div class="mt-1 flex items-center gap-1 text-xs text-base-content/60">
        <span class="font-mono">{{ org.id }}</span>
        <CopyButton :value="org.id" :label="t('common.copy')" />
      </div>
    </div>

    <AppStats :items="stats" />

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">
        {{ t('project.title') }}
      </h2>
      <div v-if="items.length" class="grid gap-3 sm:grid-cols-2">
        <NuxtLink
          v-for="project in items"
          :key="project.id"
          :to="`/o/${org.slug}/projects/${project.slug}`"
          class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
        >
          <div class="card-body">
            <h3 class="card-title text-base">
              {{ project.name }}
            </h3>
            <p class="text-sm text-base-content/60">
              {{ project.slug }}
            </p>
          </div>
        </NuxtLink>
      </div>
      <p v-else class="text-sm text-base-content/60">
        {{ t('project.empty') }}
      </p>
    </section>
  </div>
</template>
