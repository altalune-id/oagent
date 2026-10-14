<script setup lang="ts">
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import FieldError from '~/components/ui/FieldError.vue';
import { useProjectEdit } from './useProjectEdit';

const { t } = useI18n();
const { open, name, slug, busy, error, fieldErrors, show, hide, submit } = useProjectEdit();

defineExpose({ show });
</script>

<template>
  <AppModal :open="open" :title="t('project.edit')" @close="hide()">
    <form novalidate class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input
          v-model="name" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.name }"
          :disabled="busy" :aria-invalid="!!fieldErrors.name" aria-describedby="project-edit-name-error"
        >
        <FieldError id="project-edit-name-error" :message="fieldErrors.name" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.slug') }}</span>
        <input
          v-model="slug" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.slug }"
          :disabled="busy" :aria-invalid="!!fieldErrors.slug" aria-describedby="project-edit-slug-error"
        >
        <FieldError id="project-edit-slug-error" :message="fieldErrors.slug" />
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
