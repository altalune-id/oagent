<script setup lang="ts">
import AppModal from '~/components/ui/AppModal.vue'
import { useAgentEdit } from './useAgentEdit'
import ErrorText from '~/components/ui/ErrorText.vue'

const { t } = useI18n()
const { open, name, description, busy, error, show, hide, submit } = useAgentEdit()

defineExpose({ show })
</script>

<template>
  <AppModal :open="open" :title="t('agent.edit')" @close="hide()">
    <form class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input v-model="name" type="text" class="input input-sm input-bordered w-full" :disabled="busy">
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.description') }}</span>
        <textarea v-model="description" rows="3" class="textarea textarea-bordered textarea-sm w-full" :disabled="busy" />
      </label>
      <ErrorText :error="error" />
    </form>
    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="hide()">
        {{ t('common.cancel') }}
      </button>
      <button class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="submit()">
        {{ busy ? t('common.saving') : t('common.save') }}
      </button>
    </template>
  </AppModal>
</template>
