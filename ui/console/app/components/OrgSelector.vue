<script setup lang="ts">
import { computed } from 'vue'
import { useOrgsStore } from '~/stores/orgs'
import type { SelectOption } from '~/components/primitives/types'

const orgs = useOrgsStore()
const route = useRoute()
const org = computed(() => route.params.org as string | undefined)

const options = computed<SelectOption[]>(() =>
  orgs.items.map(item => ({ value: item.slug, label: item.name })),
)

function onChange(slug: string) {
  if (slug && slug !== org.value) {
    return navigateTo(`/o/${slug}`)
  }
}
</script>

<template>
  <AppSelect
    :model-value="org ?? ''"
    :options="options"
    placeholder="Select organization"
    aria-label="Organization"
    @change="onChange"
  />
</template>
