<script setup lang="ts">
import AppStats from '~/components/ui/AppStats.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'
import type { StatItem } from '~/components/ui/types'

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const org = computed(() => orgs.bySlug(route.params.org as string))

const stats = computed<StatItem[]>(() => [
  { label: t('nav.projects'), value: org.value ? projects.forOrg(org.value.id).length : 0 },
  { label: t('stat.agents'), value: 0 },
  { label: t('stat.members'), value: 1 },
])

useHead({ title: () => `${org.value?.name ?? t('org.title')} · oagent` })
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">{{ org.name }}</h1>
      <p class="text-sm text-base-content/60">{{ t('nav.overview') }}</p>
    </div>
    <AppStats :items="stats" />
  </div>
</template>
