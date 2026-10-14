<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue';
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import FieldError from '~/components/ui/FieldError.vue';
import { useAgentCreate } from './useAgentCreate';

const { orgId, projectId, label } = defineProps<{ orgId: string, projectId: string, label?: string }>();

const { t } = useI18n();
const { open, name, slug, description, busy, error, fieldErrors, show, hide, submit } = useAgentCreate(orgId, projectId);
</script>

<template>
  <AppButton size="sm" @click="show()">
    <Icon name="lucide:plus" size="16" />
    {{ label ?? t('agent.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('agent.new')" @close="hide()">
    <form novalidate class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input
          v-model="name" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.name }"
          placeholder="Support Bot" :disabled="busy" :aria-invalid="!!fieldErrors.name" aria-describedby="agent-name-error"
        >
        <FieldError id="agent-name-error" :message="fieldErrors.name" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.slug') }}</span>
        <input
          v-model="slug" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.slug }"
          placeholder="support-bot (generated when empty)" :disabled="busy" :aria-invalid="!!fieldErrors.slug" aria-describedby="agent-slug-error"
        >
        <FieldError id="agent-slug-error" :message="fieldErrors.slug" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.description') }}</span>
        <textarea
          v-model="description" rows="3" class="textarea textarea-bordered textarea-sm w-full" :class="{ 'textarea-error': fieldErrors.description }"
          placeholder="What this agent does" :disabled="busy" :aria-invalid="!!fieldErrors.description" aria-describedby="agent-description-error"
        />
        <FieldError id="agent-description-error" :message="fieldErrors.description" />
      </label>
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
