<script setup lang="ts">
import AppStats from '~/components/ui/AppStats.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'
import type { StatItem } from '~/components/ui/types'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))

const stats = computed<StatItem[]>(() => [
  { label: 'Projects', value: org.value ? projects.forOrg(org.value.id).length : 0 },
  { label: 'Agents', value: 0 },
  { label: 'Members', value: 1 },
])

useHead({ title: () => `${org.value?.name ?? 'Organization'} · oagent` })
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">{{ org.name }}</h1>
      <p class="text-sm text-base-content/60">Overview</p>
    </div>
    <AppStats :items="stats" />
  </div>
</template>
