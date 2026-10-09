<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue'
import AppModal from '~/components/ui/AppModal.vue'
import scopes from '@shared/scope/scopes.json'
import { useProjectsStore } from '~/stores/projects'
import { useAPIKeyCreate } from './useAPIKeyCreate'

const { orgId } = defineProps<{ orgId: string }>()

const projects = useProjectsStore()
const create = useAPIKeyCreate(orgId)
const { open, kind, name, scopes: selectedScopes, allProjects, projectId, projectIds, expiresInHours, busy, error, plaintext } = create

const availableScopes = computed(() => scopes.filter(s => kind.value === 'org' || s.level === 'project'))
const orgProjects = computed(() => projects.forOrg(orgId))

const EXPIRY = [
  { value: 168, label: '7 days' },
  { value: 720, label: '30 days' },
  { value: 2160, label: '90 days' },
  { value: 8760, label: '365 days' },
]

async function copy() {
  if (import.meta.client) {
    await navigator.clipboard.writeText(plaintext.value)
  }
}
</script>

<template>
  <AppButton size="sm" @click="create.show()">
    New API key
  </AppButton>

  <AppModal :open="open" title="New API key" @close="create.hide()">
    <div v-if="plaintext" class="space-y-3">
      <p class="text-sm text-warning">
        Copy this secret now — it will not be shown again.
      </p>
      <code class="block rounded-box bg-base-200 p-3 text-xs break-all">{{ plaintext }}</code>
      <button class="btn btn-sm" type="button" @click="copy">
        Copy
      </button>
    </div>

    <form v-else class="space-y-3" @submit.prevent="create.submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">Name</span>
        <input v-model="name" type="text" class="input input-sm input-bordered w-full" placeholder="deploy" :disabled="busy">
      </label>

      <div class="flex gap-4">
        <label class="flex items-center gap-2 text-sm"><input v-model="kind" type="radio" value="org" class="radio radio-sm" :disabled="busy">Org key</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="kind" type="radio" value="project" class="radio radio-sm" :disabled="busy">Project key</label>
      </div>

      <label v-if="kind === 'project'" class="block">
        <span class="mb-1 block text-xs text-base-content/60">Project</span>
        <select v-model="projectId" class="select select-sm select-bordered w-full" :disabled="busy">
          <option value="" disabled>
            Select project
          </option>
          <option v-for="p in orgProjects" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select>
      </label>

      <div v-else class="space-y-2">
        <label class="flex items-center gap-2 text-sm"><input v-model="allProjects" type="checkbox" class="checkbox checkbox-sm" :disabled="busy">All projects</label>
        <div v-if="!allProjects" class="max-h-32 space-y-1 overflow-auto rounded-box border border-base-300 p-2">
          <label v-for="p in orgProjects" :key="p.id" class="flex items-center gap-2 text-sm">
            <input v-model="projectIds" type="checkbox" :value="p.id" class="checkbox checkbox-xs" :disabled="busy">{{ p.name }}
          </label>
        </div>
      </div>

      <div>
        <span class="mb-1 block text-xs text-base-content/60">Scopes</span>
        <div class="grid grid-cols-2 gap-1">
          <label v-for="s in availableScopes" :key="s.name" class="flex items-center gap-2 text-sm">
            <input v-model="selectedScopes" type="checkbox" :value="s.name" class="checkbox checkbox-xs" :disabled="busy">{{ s.name }}
          </label>
        </div>
      </div>

      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">Expires</span>
        <select v-model.number="expiresInHours" class="select select-sm select-bordered w-full" :disabled="busy">
          <option v-for="e in EXPIRY" :key="e.value" :value="e.value">
            {{ e.label }}
          </option>
        </select>
      </label>

      <p v-if="error" class="text-sm text-error">
        {{ error }}
      </p>
    </form>

    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="create.hide()">
        {{ plaintext ? 'Done' : 'Cancel' }}
      </button>
      <button v-if="!plaintext" class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="create.submit()">
        {{ busy ? 'Creating...' : 'Create' }}
      </button>
    </template>
  </AppModal>
</template>
