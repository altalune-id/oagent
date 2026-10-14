<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue';
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import FieldError from '~/components/ui/FieldError.vue';
import SecretField from '~/components/ui/SecretField.vue';
import { webhookEvents } from '~/components/webhook/events';
import { useWebhookCreate } from './useWebhookCreate';

const { orgId, projectId } = defineProps<{ orgId: string, projectId: string }>();

const { t } = useI18n();
const create = useWebhookCreate(orgId, projectId);
const { open, url, description, eventTypes, busy, error, fieldErrors, secret, pingBusy, pingError, pinged } = create;
</script>

<template>
  <AppButton size="sm" @click="create.show()">
    <Icon name="lucide:plus" size="16" />
    {{ t('webhooks.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('webhooks.new')" @close="create.hide()">
    <div v-if="secret" class="space-y-3">
      <p class="text-sm text-warning">
        {{ t('webhooks.secret_warning') }}
      </p>
      <SecretField :value="secret" :label="t('common.copy')" />
      <div class="flex items-center gap-2">
        <button class="btn btn-outline btn-sm" type="button" :disabled="pingBusy" @click="create.ping()">
          <Icon name="lucide:send" size="16" />
          {{ pinged ? t('webhooks.tested') : t('webhooks.test') }}
        </button>
      </div>
      <ErrorText :error="pingError" />
    </div>

    <form v-else novalidate class="space-y-3" @submit.prevent="create.submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('webhooks.url') }}</span>
        <input
          v-model="url" type="url" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.url }"
          placeholder="https://example.com/hook" :disabled="busy" :aria-invalid="!!fieldErrors.url" aria-describedby="webhook-url-error"
        >
        <FieldError id="webhook-url-error" :message="fieldErrors.url" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.description') }}</span>
        <input
          v-model="description" type="text" class="input input-sm input-bordered w-full" :class="{ 'input-error': fieldErrors.description }"
          :disabled="busy" :aria-invalid="!!fieldErrors.description" aria-describedby="webhook-description-error"
        >
        <FieldError id="webhook-description-error" :message="fieldErrors.description" />
      </label>
      <div>
        <span class="mb-1 block text-xs text-base-content/60">{{ t('webhooks.events') }}</span>
        <div class="space-y-1">
          <label v-for="event in webhookEvents" :key="event.name" class="flex items-center gap-2 text-sm">
            <input v-model="eventTypes" type="checkbox" :value="event.name" class="checkbox checkbox-xs" :disabled="busy">
            {{ t(event.labelKey) }}
          </label>
        </div>
        <FieldError id="webhook-event-types-error" :message="fieldErrors.event_types" />
      </div>
      <ErrorText :error="error" />
    </form>

    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="create.hide()">
        {{ secret ? t('common.done') : t('common.cancel') }}
      </button>
      <button v-if="!secret" class="btn btn-primary btn-sm" type="button" :disabled="busy || !url.trim() || eventTypes.length === 0" @click="create.submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
