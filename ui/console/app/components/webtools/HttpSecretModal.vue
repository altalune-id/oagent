<script setup lang="ts">
import type { useHttpSecretForm } from './useHttpSecretForm';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { describedBy } from '~/lib/names/names';

const { form } = defineProps<{ form: ReturnType<typeof useHttpSecretForm> }>();

const { t } = useI18n();
const uid = useId();
const ids = { host: `${uid}-host`, header: `${uid}-header`, value: `${uid}-value` };
const { open, editing, host, header, value, errors, formError, busy } = form;
</script>

<template>
  <AppModal :open="open" :title="editing ? t('webtools.replace_value') : t('webtools.add_secret')" @close="form.hide()">
    <form novalidate class="flex flex-col gap-3" @submit.prevent="form.submit()">
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('webtools.secret_host') }}</span>
          <input v-model="host" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.host }" placeholder="api.example.com" autocomplete="off" :readonly="!!editing" :aria-invalid="!!errors.host" :aria-describedby="describedBy(`${ids.host}-hint`, !!errors.host && `${ids.host}-err`)">
        </label>
        <p :id="`${ids.host}-hint`" class="text-xs text-base-content/70">
          {{ t('webtools.host_punycode') }}
        </p>
        <FieldError :id="`${ids.host}-err`" :message="errors.host" />
      </div>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('webtools.secret_header') }}</span>
          <input v-model="header" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.header }" placeholder="X-Api-Key" autocomplete="off" :readonly="!!editing" :aria-invalid="!!errors.header" :aria-describedby="describedBy(`${ids.header}-hint`, !!errors.header && `${ids.header}-err`)">
        </label>
        <p :id="`${ids.header}-hint`" class="text-xs text-base-content/70">
          {{ t('webtools.secret_header_hint') }}
        </p>
        <FieldError :id="`${ids.header}-err`" :message="errors.header" />
      </div>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('webtools.secret_value') }}</span>
          <input v-model="value" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.value }" :aria-invalid="!!errors.value" :aria-describedby="describedBy(`${ids.value}-hint`, !!errors.value && `${ids.value}-err`)">
        </label>
        <p :id="`${ids.value}-hint`" class="text-xs text-base-content/70">
          {{ t('secret.write_only') }}
        </p>
        <FieldError :id="`${ids.value}-err`" :message="errors.value" />
      </div>
      <ErrorText :error="formError" />
    </form>

    <template #footer>
      <AppButton variant="plain" size="sm" @click="form.hide()">
        {{ t('common.cancel') }}
      </AppButton>
      <AppButton size="sm" :busy="busy" @click="form.submit()">
        {{ t('webtools.save_secret') }}
      </AppButton>
    </template>
  </AppModal>
</template>
