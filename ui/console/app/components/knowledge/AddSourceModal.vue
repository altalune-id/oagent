<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { describedBy } from '~/lib/names/names';
import { maxPagesLimit, maxPagesValid, websiteValid } from './knowledge';

const { open } = defineProps<{ open: boolean }>();
const emit = defineEmits<{ close: [], save: [url: string, maxPages: number] }>();

const { t } = useI18n();
const uid = useId();
const kind = ref<'website' | 'document'>('website');
const url = ref('');
const maxPages = ref(50);
const tried = ref(false);

const errors = computed(() => ({
  url: tried.value && !websiteValid(url.value) ? t('knowledge.url_invalid') : '',
  maxPages: tried.value && !maxPagesValid(maxPages.value) ? t('knowledge.max_pages_invalid', { max: maxPagesLimit }) : '',
}));

watch(() => open, (now) => {
  if (now) {
    kind.value = 'website';
    url.value = '';
    maxPages.value = 50;
    tried.value = false;
  }
});

function save() {
  tried.value = true;
  if (errors.value.url || errors.value.maxPages) {
    return;
  }
  emit('save', url.value.trim(), maxPages.value);
}
</script>

<template>
  <AppModal :open="open" :title="t('knowledge.add_title')" @close="emit('close')">
    <div class="flex flex-col gap-4">
      <div class="tabs tabs-box tabs-sm" role="radiogroup" :aria-label="t('knowledge.source_type')">
        <label class="tab" :class="{ 'tab-active': kind === 'website' }">
          <input v-model="kind" type="radio" :name="`${uid}-kind`" value="website">
          {{ t('knowledge.kind_website') }}
        </label>
        <label class="tab" :class="{ 'tab-active': kind === 'document' }">
          <input v-model="kind" type="radio" :name="`${uid}-kind`" value="document">
          {{ t('knowledge.kind_document') }}
        </label>
      </div>

      <form v-if="kind === 'website'" novalidate class="flex flex-col gap-3" @submit.prevent="save()">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('knowledge.url') }}</span>
          <input v-model="url" type="url" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.url }" placeholder="https://bank.example.id/promo" :aria-invalid="!!errors.url" :aria-describedby="describedBy(`${uid}-url-hint`, !!errors.url && `${uid}-url-err`)">
        </label>
        <p :id="`${uid}-url-hint`" class="-mt-2 text-xs text-base-content/70">
          {{ t('knowledge.url_hint') }}
        </p>
        <FieldError :id="`${uid}-url-err`" :message="errors.url" />
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('knowledge.max_pages') }}</span>
          <input v-model.number="maxPages" type="number" min="1" :max="maxPagesLimit" class="input input-sm w-32 tabular-nums" :class="{ 'input-error': errors.maxPages }" :aria-invalid="!!errors.maxPages" :aria-describedby="describedBy(!!errors.maxPages && `${uid}-max-err`)">
        </label>
        <FieldError :id="`${uid}-max-err`" :message="errors.maxPages" />
      </form>

      <div v-else class="flex flex-col gap-2">
        <label class="flex flex-col gap-1">
          <span class="label text-ui font-semibold text-base-content">{{ t('knowledge.file') }}</span>
          <input type="file" class="file-input file-input-sm w-full" accept=".pdf,.docx,.md,.txt" multiple disabled :aria-describedby="`${uid}-types ${uid}-storage`">
        </label>
        <p :id="`${uid}-types`" class="text-xs text-base-content/70">
          {{ t('knowledge.file_types') }}
        </p>
        <p :id="`${uid}-storage`" role="note" class="flex items-center gap-1.5 text-ui">
          <Icon name="lucide:hard-drive" size="16" class="shrink-0 text-warning" aria-hidden="true" />
          {{ t('knowledge.storage_off') }}
        </p>
      </div>
    </div>

    <template #footer>
      <AppButton variant="plain" size="sm" @click="emit('close')">
        {{ t('common.cancel') }}
      </AppButton>
      <AppButton size="sm" :disabled="kind === 'document'" @click="save()">
        {{ t('knowledge.add') }}
      </AppButton>
    </template>
  </AppModal>
</template>
