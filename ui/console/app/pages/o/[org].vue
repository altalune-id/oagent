<script setup lang="ts">
import AppSidebar from '~/components/layout/AppSidebar.vue'
import ProjectSidebar from '~/components/project/ProjectSidebar.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
const projects = useProjectsStore()
await orgs.ensureLoaded()

if (orgs.error) {
  throw createError({ statusCode: 502, statusMessage: t('error.load_orgs'), fatal: true })
}

const org = computed(() => orgs.bySlug(route.params.org as string))

if (!org.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.org_not_found'), fatal: true })
}

const projectSlug = computed(() => route.params.project as string | undefined)

await projects.ensureLoaded(org.value.id)
</script>

<template>
  <div class="flex flex-1">
    <ProjectSidebar v-if="projectSlug" />
    <AppSidebar v-else />
    <main class="min-w-0 flex-1 p-6">
      <NuxtPage />
    </main>
  </div>
</template>
