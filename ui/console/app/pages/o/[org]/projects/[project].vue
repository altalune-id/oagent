<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
)

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: 'Project not found', fatal: true })
}

useHead({ title: () => `${project.value?.name ?? 'Project'} · oagent` })
</script>

<template>
  <div v-if="project" class="space-y-4">
    <div>
      <NuxtLink :to="`/o/${org?.slug}/projects`" class="link text-sm text-base-content/60">
        Projects
      </NuxtLink>
      <h1 class="text-2xl font-bold">{{ project.name }}</h1>
      <p class="text-sm text-base-content/60">{{ project.slug }}</p>
    </div>
    <div class="rounded-box border border-base-300 bg-base-100 p-6 text-sm text-base-content/60">
      Agent configuration will live here.
    </div>
  </div>
</template>
