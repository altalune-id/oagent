<script setup lang="ts">
import type { useSourceCreate } from './useSourceCreate';
import type { KnowledgeSource } from '~/stores/knowledge';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { formatBytes } from '#base/lib/format/bytes';
import { describedBy } from '~/lib/names/names';
import { useKnowledgeStore } from '~/stores/knowledge';

const { form, storageConfigured } = defineProps<{ form: ReturnType<typeof useSourceCreate>, storageConfigured: boolean }>();
const emit = defineEmits<{ created: [source: KnowledgeSource] }>();

const { t, locale } = useI18n();
const uid = useId();
const store = useKnowledgeStore();
const { open, kind, name, seedUrl, busy, error, errors } = form;
const uploadOff = computed(() => kind.value === 'upload' && !storageConfigured);

async function save() {
  const source = await form.submit();
  if (source) {
    emit('created', source);
  }
}
</script>

<template>
  <AppModal :open="open" :title="t('knowledge.add_title')" @close="form.hide()">
    <form :id="`${uid}-form`" novalidate class="flex flex-col gap-4" @submit.prevent="save()">
      <div class="tabs tabs-box tabs-sm w-full sm:w-fit" role="radiogroup" :aria-label="t('knowledge.source_type')">
        <label class="tab flex-1 gap-1.5 sm:flex-none" :class="{ 'tab-active': kind === 'url' }">
          <input v-model="kind" type="radio" :name="`${uid}-kind`" value="url">
          <Icon name="lucide:globe" size="14" aria-hidden="true" />
          {{ t('knowledge.kind_url') }}
        </label>
        <label class="tab flex-1 gap-1.5 sm:flex-none" :class="{ 'tab-active': kind === 'upload' }">
          <input v-model="kind" type="radio" :name="`${uid}-kind`" value="upload">
          <Icon name="lucide:file-up" size="14" aria-hidden="true" />
          {{ t('knowledge.kind_upload') }}
        </label>
      </div>

      <template v-if="kind === 'url'">
        <div class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui font-semibold text-base-content">{{ t('knowledge.url') }}</span>
            <input v-model="seedUrl" type="url" inputmode="url" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.seed_url }" :placeholder="t('knowledge.url_placeholder')" :aria-invalid="!!errors.seed_url" :aria-describedby="describedBy(`${uid}-url-hint`, !!errors.seed_url && `${uid}-url-err`)">
          </label>
          <p :id="`${uid}-url-hint`" class="label whitespace-normal text-xs text-base-content/70">
            {{ t('knowledge.url_hint') }}
          </p>
          <FieldError :id="`${uid}-url-err`" :message="errors.seed_url" />
        </div>
        <div class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui font-semibold text-base-content">{{ t('knowledge.name_optional') }}</span>
            <input v-model="name" type="text" class="input input-sm w-full" :class="{ 'input-error': errors.name }" :aria-invalid="!!errors.name" :aria-describedby="describedBy(`${uid}-name-hint`, !!errors.name && `${uid}-name-err`)">
          </label>
          <p :id="`${uid}-name-hint`" class="label whitespace-normal text-xs text-base-content/70">
            {{ t('knowledge.name_default') }}
          </p>
          <FieldError :id="`${uid}-name-err`" :message="errors.name" />
        </div>
        <p class="flex items-start gap-1.5 text-xs text-base-content/70">
          <Icon name="lucide:info" size="14" class="mt-0.5 shrink-0" aria-hidden="true" />
          {{ t('knowledge.crawl_limits') }}
        </p>
      </template>

      <template v-else>
        <div class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui font-semibold text-base-content">{{ t('common.name') }}</span>
            <input v-model="name" type="text" class="input input-sm w-full" :class="{ 'input-error': errors.name }" :placeholder="t('knowledge.name_placeholder')" :aria-invalid="!!errors.name" :aria-describedby="describedBy(!!errors.name && `${uid}-upload-name-err`)">
          </label>
          <FieldError :id="`${uid}-upload-name-err`" :message="errors.name" />
        </div>
        <p v-if="storageConfigured" class="text-xs text-base-content/70">
          {{ t('knowledge.upload_next', { max: formatBytes(store.maxUploadBytes, locale) }) }}
        </p>
        <p v-else role="note" class="flex items-start gap-1.5 text-ui">
          <Icon name="lucide:hard-drive" size="16" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
          {{ t('knowledge.storage_off') }}
        </p>
      </template>
      <ErrorText :error="error" />
    </form>

    <template #footer>
      <AppButton variant="plain" size="sm" :disabled="busy" @click="form.hide()">
        {{ t('common.cancel') }}
      </AppButton>
      <AppButton type="submit" :form="`${uid}-form`" size="sm" :busy="busy" :disabled="uploadOff">
        {{ t('knowledge.add') }}
      </AppButton>
    </template>
  </AppModal>
</template>
