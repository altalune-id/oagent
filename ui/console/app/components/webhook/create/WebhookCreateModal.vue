<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import { webhookEvents } from '~/components/webhook/events';
import { useWebhookCreate } from './useWebhookCreate';

const { orgId, projectId } = defineProps<{ orgId: string, projectId: string }>();

const { t } = useI18n();
const ids = { url: useId(), urlErr: useId(), description: useId(), descriptionErr: useId(), eventsErr: useId() };
const create = useWebhookCreate(orgId, projectId);
const { open, url, description, eventTypes, busy, error, fieldErrors, secret, pingBusy, pingError, pinged } = create;
</script>

<template>
  <AppButton size="sm" @click="create.show()">
    <Icon name="lucide:plus" size="16" aria-hidden="true" />
    {{ t('webhooks.new') }}
  </AppButton>

  <AppModal :open="open" :title="t('webhooks.new')" :dismissible="!secret" @close="create.hide()">
    <div v-if="secret" class="space-y-3">
      <p class="flex items-start gap-1.5 text-ui text-base-content">
        <Icon name="lucide:triangle-alert" size="16" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        {{ t('webhooks.secret_warning') }}
      </p>
      <SecretField :value="secret" :label="t('common.copy')" />
      <div class="flex items-center gap-2">
        <AppButton variant="plain" size="sm" :disabled="pingBusy" @click="create.ping()">
          <Icon name="lucide:send" size="16" aria-hidden="true" />
          {{ pinged ? t('webhooks.tested') : t('webhooks.test') }}
        </AppButton>
      </div>
      <ErrorText :error="pingError" />
    </div>

    <form v-else novalidate class="space-y-1" @submit.prevent="create.submit()">
      <div class="fieldset">
        <label :for="ids.url" class="label text-ui text-base-content">{{ t('webhooks.url') }}</label>
        <input
          :id="ids.url" v-model="url" type="url" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.url }"
          placeholder="https://example.com/hook" :disabled="busy" :aria-invalid="!!fieldErrors.url" :aria-describedby="ids.urlErr"
        >
        <FieldError :id="ids.urlErr" :message="fieldErrors.url" />
      </div>
      <div class="fieldset">
        <label :for="ids.description" class="label text-ui text-base-content">{{ t('common.description') }}</label>
        <input
          :id="ids.description" v-model="description" type="text" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.description }"
          :disabled="busy" :aria-invalid="!!fieldErrors.description" :aria-describedby="ids.descriptionErr"
        >
        <FieldError :id="ids.descriptionErr" :message="fieldErrors.description" />
      </div>
      <fieldset class="fieldset" :aria-describedby="ids.eventsErr">
        <legend class="fieldset-legend text-ui">
          {{ t('webhooks.events') }}
        </legend>
        <div class="space-y-1">
          <label v-for="event in webhookEvents" :key="event.name" class="flex items-center gap-2 text-ui">
            <input v-model="eventTypes" type="checkbox" :value="event.name" class="checkbox checkbox-xs rounded-sm" :disabled="busy">
            {{ t(event.labelKey) }}
          </label>
        </div>
        <FieldError :id="ids.eventsErr" :message="fieldErrors.event_types" />
      </fieldset>
      <ErrorText :error="error" />
    </form>

    <template #footer>
      <AppButton variant="ghost" size="sm" :disabled="busy" @click="create.hide()">
        {{ secret ? t('common.done') : t('common.cancel') }}
      </AppButton>
      <AppButton v-if="!secret" size="sm" :disabled="busy || !url.trim() || eventTypes.length === 0" @click="create.submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </AppButton>
    </template>
  </AppModal>
</template>
