<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'
import { useUIStore } from '~/stores/ui'

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const ui = useUIStore()
const org = computed(() => orgs.bySlug(route.params.org as string))
const items = computed(() => (org.value ? projects.forOrg(org.value.id) : []))
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <h1 class="text-2xl font-bold">
        {{ t('project.title') }}
      </h1>
      <AppButton v-if="org" size="sm" @click="ui.openProjectCreate()">
        <Icon name="lucide:plus" size="16" />
        {{ t('project.new') }}
      </AppButton>
    </div>

    <p v-if="projects.error" class="text-sm text-error">
      {{ projects.error }}
    </p>
    <div v-else-if="items.length" class="grid gap-3 sm:grid-cols-2">
      <NuxtLink
        v-for="project in items"
        :key="project.slug"
        :to="`/o/${org?.slug}/projects/${project.slug}`"
        class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
      >
        <div class="card-body">
          <h2 class="card-title text-base">{{ project.name }}</h2>
          <p class="text-sm text-base-content/60">{{ project.slug }}</p>
        </div>
      </NuxtLink>
    </div>
    <p v-else class="text-sm text-base-content/60">
      {{ t('project.empty') }}
    </p>
  </div>
</template>
