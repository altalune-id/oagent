<script setup lang="ts">
import type { useMemoryStoreCreate } from './useMemoryStoreCreate';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';

const { form } = defineProps<{ form: ReturnType<typeof useMemoryStoreCreate> }>();
const emit = defineEmits<{ created: [name: string] }>();

const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), nameHelp: useId(), apiHelp: useId() };
const { open, name, rememberApiUsers, busy, error, fieldErrors, hide } = form;

async function submit() {
  const store = await form.submit();
  if (store) {
    emit('created', store.name);
  }
}
</script>

<template>
  <AppModal :open="open" :title="t('memorystores.new')" @close="hide()">
    <form novalidate class="space-y-1" @submit.prevent="submit()">
      <div class="fieldset">
        <label :for="ids.name" class="label text-ui text-base-content">{{ t('common.name') }}</label>
        <input
          :id="ids.name" v-model="name" type="text" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.name }"
          :placeholder="t('memorystores.name_placeholder')" :disabled="busy" :aria-invalid="!!fieldErrors.name" :aria-describedby="`${ids.nameHelp} ${ids.nameErr}`"
        >
        <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
        <p :id="ids.nameHelp" class="label whitespace-normal text-xs text-base-content/70">
          {{ t('memorystores.name_help') }}
        </p>
      </div>
      <div class="fieldset">
        <label class="label cursor-pointer gap-2 text-ui text-base-content">
          <input v-model="rememberApiUsers" type="checkbox" class="checkbox checkbox-sm rounded-sm" :disabled="busy" :aria-describedby="ids.apiHelp">
          {{ t('memorystores.remember_api_users') }}
        </label>
        <p :id="ids.apiHelp" class="label whitespace-normal text-xs text-base-content/70">
          {{ t('memorystores.remember_api_users_hint') }}
        </p>
      </div>
      <ErrorText :error="error" />
    </form>
    <template #footer>
      <button class="btn btn-ghost btn-sm text-ui" type="button" :disabled="busy" @click="hide()">
        {{ t('common.cancel') }}
      </button>
      <button class="btn btn-primary btn-sm text-ui" type="button" :disabled="busy || !name.trim()" @click="submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
