<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'

const route = useRoute()
const orgs = useOrgsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))

if (!org.value) {
  throw createError({ statusCode: 404, statusMessage: 'Organization not found', fatal: true })
}
</script>

<template>
  <div class="flex flex-1">
    <AppSidebar />
    <main class="min-w-0 flex-1 p-6">
      <NuxtPage />
    </main>
  </div>
</template>
