<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))
const projectCount = computed(() => (org.value ? projects.forOrg(org.value.slug).length : 0))

useHead({ title: () => `${org.value?.name ?? 'Organization'} · oagent` })
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">{{ org.name }}</h1>
      <p class="text-sm text-base-content/60">Overview</p>
    </div>
    <div class="grid gap-3 sm:grid-cols-3">
      <div class="stat rounded-box border border-base-300 bg-base-100">
        <div class="stat-title">Projects</div>
        <div class="stat-value text-3xl">{{ projectCount }}</div>
      </div>
      <div class="stat rounded-box border border-base-300 bg-base-100">
        <div class="stat-title">Agents</div>
        <div class="stat-value text-3xl">0</div>
      </div>
      <div class="stat rounded-box border border-base-300 bg-base-100">
        <div class="stat-title">Members</div>
        <div class="stat-value text-3xl">1</div>
      </div>
    </div>
  </div>
</template>
