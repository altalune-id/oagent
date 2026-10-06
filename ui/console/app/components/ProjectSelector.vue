<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const orgs = useOrgsStore()
const projects = useProjectsStore()
const route = useRoute()
const org = computed(() => route.params.org as string | undefined)
const project = computed(() => route.params.project as string | undefined)
const items = computed(() => (org.value ? projects.forOrg(org.value) : []))

function onChange(event: Event) {
  const slug = (event.target as HTMLSelectElement).value
  if (org.value && slug) {
    return navigateTo(`/o/${org.value}/projects/${slug}`)
  }
}
</script>

<template>
  <select
    v-if="org && orgs.bySlug(org)"
    class="select select-sm w-44 border-base-300"
    :value="project ?? ''"
    aria-label="Project"
    @change="onChange"
  >
    <option value="" disabled>
      Select project
    </option>
    <option v-for="item in items" :key="item.slug" :value="item.slug">
      {{ item.name }}
    </option>
  </select>
</template>
