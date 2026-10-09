<script setup lang="ts">
import AgentCreateModal from '~/components/agent/create/AgentCreateModal.vue'
import { useAgentClient } from '~/composables/agent/useAgentClient'
import { useAgentsStore } from '~/stores/agents'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const agents = useAgentsStore()
const client = useAgentClient()

const org = computed(() => orgs.bySlug(route.params.org as string))
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
)

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: 'Project not found', fatal: true })
}

await agents.ensureLoaded(project.value.id)

const items = computed(() => (project.value ? agents.forProject(project.value.id) : []))

async function destroy(id: string) {
  try {
    await client.deleteAgent({ id })
    agents.remove(id)
  }
  catch (e) {
    agents.error = e instanceof Error ? e.message : 'Failed to delete agent'
  }
}

useHead({ title: () => `${project.value?.name ?? 'Project'} · oagent` })
</script>

<template>
  <div v-if="project" class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <NuxtLink :to="`/o/${org?.slug}/projects`" class="link text-sm text-base-content/60">
          Projects
        </NuxtLink>
        <h1 class="text-2xl font-bold">{{ project.name }}</h1>
        <p class="text-sm text-base-content/60">{{ project.slug }}</p>
      </div>
      <AgentCreateModal :org-id="org?.id ?? ''" :project-id="project.id" />
    </div>

    <p v-if="agents.error" class="text-sm text-error">
      {{ agents.error }}
    </p>
    <div v-else-if="items.length" class="overflow-x-auto">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Name</th>
            <th>Slug</th>
            <th>Description</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="agent in items" :key="agent.id">
            <td>{{ agent.name }}</td>
            <td class="text-base-content/60">{{ agent.slug }}</td>
            <td class="text-base-content/60">{{ agent.description }}</td>
            <td class="text-right">
              <button class="btn btn-ghost btn-xs text-error" @click="destroy(agent.id)">
                Delete
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="text-sm text-base-content/60">
      No agents yet.
    </p>
  </div>
</template>
