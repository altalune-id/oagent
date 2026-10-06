<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'

const route = useRoute()
const orgs = useOrgsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))

useHead({ title: () => `${org.value?.name ?? 'Organization'} settings · oagent` })
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Settings</h1>
    <div class="rounded-box border border-base-300 bg-base-100 p-6 text-sm text-base-content/60">
      Organization settings will live here.
    </div>
  </div>
</template>
