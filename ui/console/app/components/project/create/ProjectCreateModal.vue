<script setup lang="ts">
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { useProjectCreate } from './useProjectCreate';

const { orgId } = defineProps<{ orgId: string }>();

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), slug: useId(), slugErr: useId(), slugHelp: useId() };
const { open, name, slug, busy, error, fieldErrors, hide, submit } = useProjectCreate(orgId);
</script>

<template>
  <AppModal :open="open" :title="t('project.new')" @close="hide()">
    <form novalidate class="space-y-1" @submit.prevent="submit()">
      <div class="fieldset">
        <label :for="ids.name" class="fieldset-legend">{{ t('common.name') }}</label>
        <input
          :id="ids.name" v-model="name" type="text" class="input w-full" :class="{ 'input-error': fieldErrors.name }"
          :placeholder="t('project.name_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
        >
        <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
      </div>
      <div class="fieldset">
        <label :for="ids.slug" class="fieldset-legend">{{ t('common.slug') }}</label>
        <input
          :id="ids.slug" v-model="slug" type="text" class="input w-full font-mono" :class="{ 'input-error': fieldErrors.slug }"
          :placeholder="t('project.slug_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.slug" :aria-describedby="`${ids.slugHelp} ${ids.slugErr}`"
        >
        <FieldError :id="ids.slugErr" :message="fieldErrors.slug" />
        <p :id="ids.slugHelp" class="label whitespace-normal">
          {{ t('project.slug_random_help') }}
        </p>
      </div>
      <ErrorText :error="error" />
    </form>
    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="hide()">
        {{ t('common.cancel') }}
      </button>
      <button class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
