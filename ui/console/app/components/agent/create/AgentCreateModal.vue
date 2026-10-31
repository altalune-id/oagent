<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { useAgentCreate } from './useAgentCreate';

const { orgId, projectId, label } = defineProps<{ orgId: string, projectId: string, label?: string }>();

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), slug: useId(), slugErr: useId(), slugHelp: useId(), description: useId(), descriptionErr: useId() };
const { open, name, slug, description, busy, error, fieldErrors, show, hide, submit } = useAgentCreate(orgId, projectId);
</script>

<template>
  <AppButton size="sm" @click="show()">
    <Icon name="lucide:plus" size="16" />
    {{ label ?? t('agent.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('agent.new')" @close="hide()">
    <form novalidate class="space-y-1" @submit.prevent="submit()">
      <div class="fieldset">
        <label :for="ids.name" class="fieldset-legend">{{ t('common.name') }}</label>
        <input
          :id="ids.name" v-model="name" type="text" class="input w-full" :class="{ 'input-error': fieldErrors.name }"
          :placeholder="t('agent.name_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
        >
        <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
      </div>
      <div class="fieldset">
        <label :for="ids.slug" class="fieldset-legend">{{ t('common.slug') }}</label>
        <input
          :id="ids.slug" v-model="slug" type="text" class="input w-full font-mono" :class="{ 'input-error': fieldErrors.slug }"
          :placeholder="t('agent.slug_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.slug" :aria-describedby="`${ids.slugHelp} ${ids.slugErr}`"
        >
        <FieldError :id="ids.slugErr" :message="fieldErrors.slug" />
        <p :id="ids.slugHelp" class="label whitespace-normal">
          {{ t('agent.slug_random_help') }}
        </p>
      </div>
      <div class="fieldset">
        <label :for="ids.description" class="fieldset-legend">{{ t('common.description') }}</label>
        <textarea
          :id="ids.description" v-model="description" rows="3" class="textarea w-full" :class="{ 'textarea-error': fieldErrors.description }"
          :placeholder="t('agent.description_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.description" :aria-describedby="ids.descriptionErr"
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
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
