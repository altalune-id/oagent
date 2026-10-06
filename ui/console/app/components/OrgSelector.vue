<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'

const orgs = useOrgsStore()
const route = useRoute()
const org = computed(() => route.params.org as string | undefined)

function onChange(event: Event) {
  const slug = (event.target as HTMLSelectElement).value
  if (slug && slug !== org.value) {
    return navigateTo(`/o/${slug}`)
  }
}
</script>

<template>
  <select
    class="select select-sm w-44 border-base-300"
    :value="org ?? ''"
    aria-label="Organization"
    @change="onChange"
  >
    <option value="" disabled>
      Select organization
    </option>
    <option v-for="item in orgs.items" :key="item.slug" :value="item.slug">
      {{ item.name }}
    </option>
  </select>
</template>
