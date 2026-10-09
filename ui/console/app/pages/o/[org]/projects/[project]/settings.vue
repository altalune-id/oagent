<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
const projects = useProjectsStore()

const org = computed(() => orgs.bySlug(route.params.org as string))
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
)

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true })
}

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('settings.title')} · oagent` })
</script>

<template>
  <div v-if="project" class="space-y-6">
    <h1 class="text-2xl font-bold">
      {{ t('settings.title') }}
    </h1>

    <div class="rounded-box border border-base-300 bg-base-100 p-4">
      <dl class="grid gap-2 text-sm sm:grid-cols-[8rem_1fr]">
        <dt class="text-base-content/60">
          {{ t('common.name') }}
        </dt>
        <dd>{{ project.name }}</dd>
        <dt class="text-base-content/60">
          {{ t('common.slug') }}
        </dt>
        <dd>{{ project.slug }}</dd>
      </dl>
    </div>

    <p class="text-sm text-base-content/50">
      {{ t('project.settings_empty') }}
    </p>
  </div>
</template>
