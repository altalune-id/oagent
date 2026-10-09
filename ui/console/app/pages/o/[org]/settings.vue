<script setup lang="ts">
import APIKeyCreateModal from '~/components/apikey/create/APIKeyCreateModal.vue'
import APIKeyList from '~/components/apikey/list/APIKeyList.vue'
import { useOrgsStore } from '~/stores/orgs'

const route = useRoute()
const orgs = useOrgsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))
</script>

<template>
  <div v-if="org" class="space-y-6">
    <h1 class="text-2xl font-bold">
      Settings
    </h1>

    <section class="space-y-3">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold">
            API keys
          </h2>
          <p class="text-sm text-base-content/60">
            Machine credentials for the API, MCP, and CLI. Secrets are shown once.
          </p>
        </div>
        <APIKeyCreateModal :org-id="org.id" />
      </div>
      <APIKeyList :org-id="org.id" />
    </section>
  </div>
</template>
