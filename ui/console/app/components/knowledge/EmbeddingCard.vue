<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import type { EmbeddingChoice } from './knowledge';
import type { ModelProvider } from '~/stores/modelProviders';
import AppButton from '#base/components/ui/AppButton.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { useToast } from '#base/components/ui/toast';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useKnowledgeStore } from '~/stores/knowledge';
import { embeddingChoices, reembeds } from './knowledge';

const { projectId, providers, canEdit, providersLink } = defineProps<{ projectId: string, providers: ModelProvider[], canEdit: boolean, providersLink: string }>();
const emit = defineEmits<{ saved: [] }>();

const { t } = useI18n();
const uid = useId();
const store = useKnowledgeStore();
const toast = useToast();
const errorMessage = useErrorMessage();
const editing = ref(false);
const picked = ref('');
const busy = ref(false);
const fieldError = ref<ShownError | null>(null);
const changeButton = ref<{ $el: HTMLElement }>();
const modelSelect = ref<HTMLSelectElement>();

const choices = computed(() => embeddingChoices(providers));
const keyOf = (c: { providerId: string, model: string }) => `${c.providerId}|${c.model}`;
const chosen = computed(() => choices.value.find(c => c.priced && keyOf(c) === picked.value));
const unchanged = computed(() => !!store.embedding && !store.embedding.providerMissing && keyOf(store.embedding) === picked.value);
const current = computed(() => store.embedding && { ...store.embedding, providerName: providers.find(p => p.id === store.embedding!.providerId)?.name ?? '—' });
const showForm = computed(() => canEdit && (editing.value || !store.embedding || !!store.embedding.providerMissing));

async function close() {
  editing.value = false;
  await nextTick();
  changeButton.value?.$el.focus();
}

async function apply(c: EmbeddingChoice) {
  await store.setEmbedding(projectId, c.providerId, c.model);
  toast.push('success', t('knowledge.embedding_saved', { model: c.model }));
  emit('saved');
  await close();
}

const reembed = confirmHandle<EmbeddingChoice>(apply, e => errorMessage(e, 'error.set_embedding_model'));

// The dialog hands focus back to its opener when it closes, and that button is gone by then.
watch(() => reembed.target.value, async (now, before) => {
  if (!now && before && !editing.value) {
    await nextTick();
    changeButton.value?.$el.focus();
  }
});

async function edit() {
  picked.value = store.embedding ? keyOf(store.embedding) : '';
  fieldError.value = null;
  editing.value = true;
  await nextTick();
  modelSelect.value?.focus();
}

async function save() {
  const c = chosen.value;
  if (!c) {
    return;
  }
  if (reembeds(store.embedding, c, store.sources.length)) {
    reembed.ask(c);
    return;
  }
  busy.value = true;
  fieldError.value = null;
  try {
    await apply(c);
  } catch (e) {
    fieldError.value = errorMessage(e, 'error.set_embedding_model');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="flex flex-col gap-3 rounded-box border border-base-300 bg-base-100 p-4" :aria-labelledby="`${uid}-title`">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h2 :id="`${uid}-title`" class="text-ui font-semibold">
        {{ t('knowledge.embedding_title') }}
      </h2>
      <AppButton v-if="canEdit && store.embedding && !editing" ref="changeButton" variant="ghost" size="sm" @click="edit()">
        <Icon name="lucide:pencil" size="14" aria-hidden="true" />
        {{ t('knowledge.embedding_change') }}
      </AppButton>
    </div>

    <div v-if="store.embedding?.providerMissing" role="alert" class="alert alert-soft alert-warning items-start text-ui text-base-content">
      <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ t('knowledge.embedding_provider_missing') }}</span>
    </div>
    <p v-if="current" data-embedding class="flex flex-wrap items-center gap-x-2 gap-y-1 text-ui">
      <Icon name="lucide:binary" size="16" class="shrink-0" aria-hidden="true" />
      <span class="font-mono text-xs break-all">{{ current.model }}</span>
      <span class="text-base-content/70">{{ t('knowledge.embedding_line', { provider: current.providerName, dims: current.dims }) }}</span>
    </p>
    <div v-else role="note" class="alert alert-soft alert-warning items-start text-ui text-base-content">
      <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ canEdit ? t('knowledge.embedding_none') : t('knowledge.embedding_none_readonly') }}</span>
    </div>

    <template v-if="showForm">
      <p v-if="!choices.length" class="text-ui text-base-content/70">
        {{ t('knowledge.embedding_no_models') }}
        <NuxtLink :to="providersLink" class="link">
          {{ t('providers.title') }}
        </NuxtLink>.
      </p>
      <form v-else novalidate class="flex flex-col gap-2 sm:flex-row sm:items-start" @submit.prevent="save()">
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('knowledge.embedding_model') }}</span>
            <select ref="modelSelect" v-model="picked" class="select select-sm w-full text-ui" :class="{ 'select-error': fieldError }" :aria-invalid="!!fieldError" :aria-describedby="`${uid}-hint ${fieldError ? `${uid}-error` : ''}`">
              <option value="" disabled>
                {{ t('knowledge.embedding_choose') }}
              </option>
              <option v-for="c in choices" :key="keyOf(c)" :value="keyOf(c)" :disabled="!c.priced">
                {{ c.priced ? t('knowledge.embedding_option', { model: c.model, provider: c.providerName }) : t('knowledge.embedding_option_unpriced', { model: c.model, provider: c.providerName }) }}
              </option>
            </select>
          </label>
          <p :id="`${uid}-hint`" class="text-xs text-base-content/70">
            {{ t('knowledge.embedding_hint') }}
          </p>
          <FieldError :id="`${uid}-error`" :message="fieldError?.message" />
        </div>
        <div class="flex gap-2 sm:pt-7">
          <AppButton v-if="editing" variant="plain" size="sm" :disabled="busy" @click="close()">
            {{ t('common.cancel') }}
          </AppButton>
          <AppButton type="submit" size="sm" :busy="busy" :disabled="!chosen || unchanged">
            {{ t('knowledge.embedding_save') }}
          </AppButton>
        </div>
      </form>
    </template>

    <ConfirmModal
      :handle="reembed"
      :title="t('knowledge.reembed_title')"
      :body="t('knowledge.reembed_body', { model: reembed.target.value?.model ?? '' })"
      :confirm-label="t('knowledge.reembed_confirm')"
      :cancel-label="t('common.cancel')"
    />
  </section>
</template>
