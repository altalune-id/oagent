<script setup lang="ts">
import AppSelect from '~/components/ui/AppSelect.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'
import type { SelectOption } from '~/components/ui/types'

const orgs = useOrgsStore()
const projects = useProjectsStore()
const route = useRoute()
const org = computed(() => route.params.org as string | undefined)
const project = computed(() => route.params.project as string | undefined)

const options = computed<SelectOption[]>(() =>
  org.value ? projects.forOrg(org.value).map(item => ({ value: item.slug, label: item.name })) : [],
)

function onChange(slug: string) {
  if (org.value && slug) {
    return navigateTo(`/o/${org.value}/projects/${slug}`)
  }
}
</script>

<template>
  <AppSelect
    v-if="org && orgs.bySlug(org)"
    :model-value="project ?? ''"
    :options="options"
    placeholder="Select project"
    aria-label="Project"
    @change="onChange"
  />
</template>
