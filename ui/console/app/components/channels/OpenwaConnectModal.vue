<script setup lang="ts">
import type { useOpenwaConnect } from './useOpenwaConnect';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import CodeText from '~/components/layout/CodeText.vue';
import { describedBy } from '~/lib/names/names';

const { form, unpublished = false } = defineProps<{ form: ReturnType<typeof useOpenwaConnect>, unpublished?: boolean }>();

const { t } = useI18n();
const uid = useId();
const { open, step, baseUrl, deviceId, secret, deviceKey, errors, checking, result, webhookUrl, secretsNote } = form;
const stepKeys = ['openwa.step_webhook', 'openwa.step_keys', 'openwa.step_check'];
const before = ['openwa.before_device', 'openwa.before_webhook', 'openwa.before_key', 'openwa.before_senders', 'openwa.before_vision'];
const resultTone = { connected: 'success', no_message: 'warning', bad_signature: 'error', key_refused: 'error', device_mismatch: 'error' } as const;
</script>

<template>
  <AppModal :open="open" :title="t('openwa.connect_title')" wide @close="form.hide()">
    <div class="flex flex-col gap-4">
      <p role="note" class="flex items-center gap-1.5 text-xs text-base-content/70">
        <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
        {{ t('openwa.preview_note') }}
      </p>
      <div v-if="unpublished" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
        <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
        <span>{{ t('openwa.publish_warning') }}</span>
      </div>
      <ul class="steps steps-vertical w-full sm:steps-horizontal">
        <li v-for="(key, i) in stepKeys" :key="key" class="step text-ui" :class="{ 'step-primary': step >= i + 1 }" :aria-current="step === i + 1 ? 'step' : undefined">
          {{ t(key) }}
        </li>
      </ul>

      <div v-if="step === 1" class="flex flex-col gap-3">
        <section class="rounded-box border border-base-300 p-3" :aria-labelledby="`${uid}-before`">
          <h3 :id="`${uid}-before`" class="text-ui font-semibold">
            {{ t('openwa.before_title') }}
          </h3>
          <ul class="mt-2 flex flex-col gap-1 text-ui">
            <li v-for="key in before" :key="key" class="flex items-start gap-2">
              <span aria-hidden="true">•</span>
              <span><CodeText :text="t(key)" /></span>
            </li>
          </ul>
        </section>
        <div class="flex flex-col gap-2">
          <p class="text-ui font-semibold">
            {{ t('openwa.webhook_url') }}
          </p>
          <SecretField :value="webhookUrl" :label="t('common.copy')" />
          <p class="text-ui text-base-content/70">
            <CodeText :text="t('openwa.webhook_help')" />
          </p>
        </div>
      </div>

      <form v-else-if="step === 2" novalidate class="flex flex-col gap-3" @submit.prevent="form.next()">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('openwa.base_url') }}</span>
          <input v-model="baseUrl" type="url" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.baseUrl }" placeholder="https://openwa.example.id" :aria-invalid="!!errors.baseUrl" :aria-describedby="describedBy(!!errors.baseUrl && `${uid}-url-err`)">
        </label>
        <FieldError :id="`${uid}-url-err`" :message="errors.baseUrl" />
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('openwa.device_id') }}</span>
          <input v-model="deviceId" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.deviceId }" placeholder="dev_yasaku-rumah" :aria-invalid="!!errors.deviceId" :aria-describedby="describedBy(!!errors.deviceId && `${uid}-dev-err`)">
        </label>
        <FieldError :id="`${uid}-dev-err`" :message="errors.deviceId" />
        <p v-if="secretsNote" role="note" class="text-xs text-base-content/70">
          {{ t(secretsNote) }}
        </p>
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('openwa.secret') }}</span>
          <input v-model="secret" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.secret }" :aria-invalid="!!errors.secret" :aria-describedby="describedBy(`${uid}-secret-hint`, !!errors.secret && `${uid}-secret-err`)">
        </label>
        <p :id="`${uid}-secret-hint`" class="-mt-2 text-xs text-base-content/70">
          {{ t('openwa.secret_hint') }} {{ t('secret.write_only') }}
        </p>
        <FieldError :id="`${uid}-secret-err`" :message="errors.secret" />
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('openwa.device_key') }}</span>
          <input v-model="deviceKey" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.deviceKey }" :aria-invalid="!!errors.deviceKey" :aria-describedby="describedBy(`${uid}-key-hint`, !!errors.deviceKey && `${uid}-key-err`)">
        </label>
        <p :id="`${uid}-key-hint`" class="-mt-2 text-xs text-base-content/70">
          <CodeText :text="t('openwa.device_key_hint')" /> {{ t('secret.write_only') }}
        </p>
        <FieldError :id="`${uid}-key-err`" :message="errors.deviceKey" />
      </form>

      <div v-else class="flex flex-col gap-3" aria-live="polite">
        <p class="text-ui text-base-content/70">
          {{ t('openwa.check_help') }}
        </p>
        <p v-if="checking" role="status" class="flex items-center gap-2 text-ui text-base-content/70">
          <span class="loading loading-spinner loading-xs" aria-hidden="true" />
          {{ t('openwa.checking') }}
        </p>
        <div v-else-if="result" class="flex flex-col gap-1">
          <StatusLabel :tone="resultTone[result]" :label="t(`openwa.result_${result}`)" />
          <p class="text-ui text-base-content/70">
            <CodeText :text="t(`openwa.result_${result}_detail`)" />
          </p>
        </div>
      </div>
    </div>

    <template #footer>
      <AppButton v-if="step === 1" variant="plain" size="sm" @click="form.hide()">
        {{ t('common.cancel') }}
      </AppButton>
      <AppButton v-if="step > 1 && result !== 'connected'" variant="plain" size="sm" :disabled="checking" @click="form.back()">
        {{ t('openwa.back') }}
      </AppButton>
      <AppButton v-if="step === 3 && result !== 'connected'" variant="outline" size="sm" :disabled="checking" @click="form.finishLater()">
        {{ t('openwa.finish_later') }}
      </AppButton>
      <AppButton v-if="step < 3" size="sm" @click="form.next()">
        {{ t('openwa.next') }}
      </AppButton>
      <AppButton v-else-if="result === 'connected'" size="sm" @click="form.hide()">
        {{ t('common.done') }}
      </AppButton>
      <AppButton v-else size="sm" :busy="checking" @click="form.saveAndTest()">
        {{ t('openwa.save_test') }}
      </AppButton>
    </template>
  </AppModal>
</template>
