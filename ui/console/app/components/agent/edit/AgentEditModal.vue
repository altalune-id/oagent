<script setup lang="ts">
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { useAgentEdit } from './useAgentEdit';

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), description: useId(), descriptionErr: useId() };
const { open, name, description, busy, error, fieldErrors, show, hide, submit } = useAgentEdit();

defineExpose({ show });
</script>

<template>
  <AppModal :open="open" :title="t('agent.edit')" @close="hide()">
    <form novalidate class="space-y-1" @submit.prevent="submit()">
      <div class="fieldset">
        <label :for="ids.name" class="fieldset-legend">{{ t('common.name') }}</label>
        <input
          :id="ids.name" v-model="name" type="text" class="input w-full" :class="{ 'input-error': fieldErrors.name }"
          :disabled="busy" :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
        >
        <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
      </div>
      <div class="fieldset">
        <label :for="ids.description" class="fieldset-legend">{{ t('common.description') }}</label>
        <textarea
          :id="ids.description" v-model="description" rows="3" class="textarea w-full" :class="{ 'textarea-error': fieldErrors.description }"
          :disabled="busy" :aria-invalid="!!fieldErrors.description" :aria-describedby="ids.descriptionErr"
        />
        <FieldError :id="ids.descriptionErr" :message="fieldErrors.description" />
      </div>
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
