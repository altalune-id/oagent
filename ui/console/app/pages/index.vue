<script setup lang="ts">
import OrgCreateModal from '~/components/org/create/OrgCreateModal.vue'
import { useOrgsStore } from '~/stores/orgs'

useHead({ title: 'Organizations · oagent' })
const orgs = useOrgsStore()
await orgs.ensureLoaded()
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-6 py-10">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">Organizations</h1>
        <p class="mt-1 text-sm text-base-content/60">Pick an organization to manage its agents.</p>
      </div>
      <OrgCreateModal />
    </div>

    <p v-if="orgs.error" class="mt-6 text-sm text-error">
      {{ orgs.error }}
    </p>
    <p v-else-if="orgs.items.length === 0" class="mt-6 text-sm text-base-content/60">
      No organizations yet.
    </p>
    <div v-else class="mt-6 grid gap-3 sm:grid-cols-2">
      <NuxtLink
        v-for="org in orgs.items"
        :key="org.slug"
        :to="`/o/${org.slug}`"
        class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
      >
        <div class="card-body">
          <h2 class="card-title text-base">{{ org.name }}</h2>
          <p class="text-sm text-base-content/60">/{{ org.slug }}</p>
        </div>
      </NuxtLink>
    </div>
  </div>
</template>
