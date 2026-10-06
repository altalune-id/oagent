<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))
const items = computed(() => (org.value ? projects.forOrg(org.value.slug) : []))
</script>

<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold">Projects</h1>
    <div v-if="items.length" class="grid gap-3 sm:grid-cols-2">
      <NuxtLink
        v-for="project in items"
        :key="project.slug"
        :to="`/o/${org?.slug}/projects/${project.slug}`"
        class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
      >
        <div class="card-body">
          <h2 class="card-title text-base">{{ project.name }}</h2>
          <p class="text-sm text-base-content/60">{{ project.slug }}</p>
        </div>
      </NuxtLink>
    </div>
    <p v-else class="text-sm text-base-content/60">No projects yet.</p>
  </div>
</template>
