<script setup lang="ts">
import WebhookCreateModal from '~/components/webhook/create/WebhookCreateModal.vue';
import WebhookList from '~/components/webhook/list/WebhookList.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('webhooks.title')} · oagent` });
</script>

<template>
  <div v-if="org && project" class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">
          {{ t('webhooks.title') }}
        </h1>
        <p class="text-sm text-base-content/60">
          {{ t('webhooks.subtitle') }}
        </p>
      </div>
      <WebhookCreateModal :org-id="org.id" :project-id="project.id" />
    </div>
    <WebhookList :org-id="org.id" :project-id="project.id" />
  </div>
</template>
