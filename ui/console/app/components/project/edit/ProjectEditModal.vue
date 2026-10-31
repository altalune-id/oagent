<script setup lang="ts">
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { useProjectEdit } from './useProjectEdit';

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), slug: useId(), slugErr: useId() };
const { open, name, slug, busy, error, fieldErrors, show, hide, submit } = useProjectEdit();

defineExpose({ show });
</script>

<template>
  <AppModal :open="open" :title="t('project.edit')" @close="hide()">
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
        <label :for="ids.slug" class="fieldset-legend">{{ t('common.slug') }}</label>
        <input
          :id="ids.slug" v-model="slug" type="text" class="input w-full font-mono" :class="{ 'input-error': fieldErrors.slug }"
          :disabled="busy" :aria-invalid="!!fieldErrors.slug" :aria-describedby="ids.slugErr"
        >
        <FieldError :id="ids.slugErr" :message="fieldErrors.slug" />
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
