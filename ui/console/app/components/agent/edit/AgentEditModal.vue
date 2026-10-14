<script setup lang="ts">
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import FieldError from '~/components/ui/FieldError.vue';
import { useAgentEdit } from './useAgentEdit';

const { t } = useI18n();
const { open, name, description, busy, error, fieldErrors, show, hide, submit } = useAgentEdit();

defineExpose({ show });
</script>

<template>
  <AppModal :open="open" :title="t('agent.edit')" @close="hide()">
    <form novalidate class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input
          v-model="name" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.name }"
          :disabled="busy" :aria-invalid="!!fieldErrors.name" aria-describedby="agent-edit-name-error"
        >
        <FieldError id="agent-edit-name-error" :message="fieldErrors.name" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.description') }}</span>
        <textarea
          v-model="description" rows="3" class="textarea textarea-bordered textarea-sm w-full" :class="{ 'textarea-error': fieldErrors.description }"
          :disabled="busy" :aria-invalid="!!fieldErrors.description" aria-describedby="agent-edit-description-error"
        />
        <FieldError id="agent-edit-description-error" :message="fieldErrors.description" />
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
