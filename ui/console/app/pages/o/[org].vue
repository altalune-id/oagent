<script setup lang="ts">
import AppSidebar from '~/components/layout/AppSidebar.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
await orgs.ensureLoaded()

if (orgs.error) {
  throw createError({ statusCode: 502, statusMessage: 'Failed to load organizations', fatal: true })
}

const org = computed(() => orgs.bySlug(route.params.org as string))

if (!org.value) {
  throw createError({ statusCode: 404, statusMessage: 'Organization not found', fatal: true })
}

await projects.ensureLoaded(org.value.id)
</script>

<template>
  <div class="flex flex-1">
    <AppSidebar />
    <main class="min-w-0 flex-1 p-6">
      <NuxtPage />
    </main>
  </div>
</template>
