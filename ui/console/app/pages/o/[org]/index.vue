<script setup lang="ts">
import AppBadge from '#base/components/ui/AppBadge.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { environmentLabelKey } from '~/components/project/environment';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const items = computed(() => (org.value ? projects.forOrg(org.value.id) : []).map((p) => {
  const env = environmentLabelKey(p.environment);
  return { ...p, tag: env === 'environment.none' ? '' : t(env) };
}));

useHead({ title: () => `${org.value?.name ?? t('org.title')} · OAgent` });
</script>

<template>
  <div v-if="org" class="flex flex-col gap-5">
    <PageHeader :title="org.name" :description="t('org.overview_subtitle')">
      <template #status>
        <p class="flex items-center gap-1 text-xs text-base-content/70">
          <span class="font-mono">{{ org.id }}</span>
          <CopyButton :value="org.id" :label="t('common.copy_label', { label: t('common.id') })" />
        </p>
      </template>
    </PageHeader>

    <Section :title="t('project.title')" flush class="max-w-2xl" data-testid="org-projects">
      <ul v-if="items.length" class="divide-y divide-base-300">
        <li v-for="project in items" :key="project.id" class="flex items-center gap-2 px-4 py-2 text-ui">
          <NuxtLink :to="`/o/${org.slug}/projects/${project.slug}`" class="link link-hover min-w-0 truncate font-medium">
            {{ project.name }}
          </NuxtLink>
          <span class="min-w-0 truncate font-mono text-xs text-base-content/70">{{ project.slug }}</span>
          <AppBadge v-if="project.tag" :label="project.tag" class="ms-auto shrink-0" />
        </li>
      </ul>
      <p v-else class="px-4 py-3 text-ui text-base-content/70">
        {{ t('project.empty') }}
      </p>
    </Section>
  </div>
</template>
