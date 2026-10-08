<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue'
import AppModal from '~/components/ui/AppModal.vue'
import { useProjectCreate } from './useProjectCreate'

const { orgId, label = 'New project' } = defineProps<{ orgId: string, label?: string }>()

const { open, name, slug, busy, error, show, hide, submit } = useProjectCreate(orgId)
</script>

<template>
  <AppButton size="sm" @click="show()">
    {{ label }}
  </AppButton>

  <AppModal :open="open" title="New project" @close="hide()">
    <form class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">Name</span>
        <input
          v-model="name"
          type="text"
          class="input input-sm input-bordered w-full"
          placeholder="Support Bot"
          :disabled="busy"
        >
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">Slug</span>
        <input
          v-model="slug"
          type="text"
          class="input input-sm input-bordered w-full"
          placeholder="support-bot (generated when empty)"
          :disabled="busy"
        >
      </label>
      <p v-if="error" class="text-sm text-error">
        {{ error }}
      </p>
    </form>
    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="hide()">
        Cancel
      </button>
      <button class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="submit()">
        {{ busy ? 'Creating...' : 'Create' }}
      </button>
    </template>
  </AppModal>
</template>
