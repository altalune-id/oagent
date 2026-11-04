<script setup lang="ts">
import type { useProviderForm } from './useProviderForm';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { describedBy } from '~/lib/names/names';
import { hasMorePrices, mainPriceKeys, maxModels, morePriceKeys } from './useProviderForm';

const { form, readOnly = false } = defineProps<{ form: ReturnType<typeof useProviderForm>, readOnly?: boolean }>();

const { t } = useI18n();
const id = useId();
const ids = { name: `${id}-name`, url: `${id}-url`, urlHint: `${id}-url-hint`, key: `${id}-key`, keyHint: `${id}-key-hint`, models: `${id}-models` };
const { open, editing, name, baseUrl, key, replacingKey, models, errors, modelErrors, busy, urlChanged, keyShown, keyRequired } = form;

const openPrices = ref(new Set<number>());
watch(open, (shown) => {
  if (shown) {
    openPrices.value = new Set(models.value.filter(hasMorePrices).map(m => m.key));
  }
}, { immediate: true });

const title = computed(() => (editing.value ? editing.value.name : t('providers.add')));
const priceLabels = computed(() => ({ input: t('providers.price_input'), output: t('providers.price_output'), cacheRead: t('providers.price_cache_read'), cacheWrite: t('providers.price_cache_write') }));
const facts = computed(() => editing.value
  ? [
      { label: t('common.name'), value: editing.value.name },
      { label: t('providers.base_url'), value: editing.value.baseUrl, mono: true },
      { label: t('providers.api_key'), value: t('providers.key_set') },
      { label: t('providers.models'), value: editing.value.models.map(m => m.id).join(', '), mono: true },
      { label: t('providers.used_by'), value: editing.value.usedBy.join(', ') || t('providers.used_by_none') },
    ]
  : []);
</script>

<template>
  <AppModal :open="open" :title="title" wide sticky-footer @close="form.hide()">
    <KeyValue v-if="readOnly" :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })" />
    <form v-else novalidate class="flex flex-col gap-2" @submit.prevent="form.submit()">
      <fieldset class="fieldset">
        <legend class="fieldset-legend text-ui">
          {{ t('providers.connection') }}
        </legend>
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('common.name') }}</span>
          <input v-model="name" type="text" class="input input-sm w-full text-ui" :class="{ 'input-error': errors.name }" placeholder="OpenRouter" :aria-invalid="!!errors.name" :aria-describedby="describedBy(!!errors.name && `${ids.name}-err`)">
        </label>
        <FieldError :id="`${ids.name}-err`" :message="errors.name" />
        <p v-if="editing?.usedBy.length" class="text-xs text-base-content/70">
          {{ t('providers.used_by_list', { agents: editing.usedBy.join(', ') }) }}
        </p>

        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ t('providers.base_url') }}</span>
          <input v-model="baseUrl" type="url" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.baseUrl }" placeholder="https://openrouter.ai/api/v1" :aria-invalid="!!errors.baseUrl" :aria-describedby="describedBy(ids.urlHint, !!errors.baseUrl && `${ids.url}-err`)">
        </label>
        <p :id="ids.urlHint" class="label text-xs text-base-content/70">
          {{ t('providers.url_hint') }}
        </p>
        <FieldError :id="`${ids.url}-err`" :message="errors.baseUrl" />
        <div v-if="urlChanged" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
          <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
          <span>{{ t('providers.url_changed_key') }}</span>
        </div>

        <template v-if="keyShown">
          <label class="flex flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('providers.api_key') }}<span v-if="keyRequired" class="text-base-content/70"> ({{ t('validation.required') }})</span></span>
            <input v-model="key" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': errors.key }" :aria-invalid="!!errors.key" :aria-describedby="describedBy(ids.keyHint, !!errors.key && `${ids.key}-err`)" :required="keyRequired">
          </label>
          <p :id="ids.keyHint" class="label whitespace-normal text-xs text-base-content/70">
            {{ t('secret.write_only') }}
          </p>
          <FieldError :id="`${ids.key}-err`" :message="errors.key" />
        </template>
        <div v-else class="flex flex-wrap items-center gap-3">
          <StatusLabel tone="success" :label="t('providers.key_set')" />
          <AppButton variant="plain" size="xs" @click="replacingKey = true">
            {{ t('providers.replace_key') }}
          </AppButton>
        </div>
      </fieldset>

      <fieldset class="fieldset min-w-0">
        <legend class="fieldset-legend text-ui">
          {{ t('providers.models') }}
        </legend>
        <p class="text-xs text-base-content/70">
          {{ t('providers.models_hint') }}
        </p>
        <ul class="flex flex-col gap-2">
          <li v-for="m in models" :key="m.key" class="flex flex-col gap-2 rounded-box border border-base-300 p-3">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-end">
              <label class="flex min-w-0 flex-1 flex-col gap-1">
                <span class="label text-xs text-base-content">{{ t('providers.model_id') }}</span>
                <input v-model="m.id" type="text" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': modelErrors[m.key] }" placeholder="openai/gpt-4.1-mini" :aria-invalid="!!modelErrors[m.key]" :aria-describedby="describedBy(!!modelErrors[m.key] && `${ids.models}-${m.key}`)">
              </label>
              <div class="flex flex-wrap items-end gap-2">
                <label v-for="k in mainPriceKeys" :key="k" class="flex w-28 flex-col gap-1">
                  <span class="label text-xs text-base-content">{{ priceLabels[k] }}</span>
                  <span class="input input-sm w-full tabular-nums">
                    <span aria-hidden="true">$</span>
                    <input v-model="m.prices[k]" type="text" inputmode="decimal" :aria-label="t('providers.price_for', { kind: priceLabels[k], model: m.id || '…' })" :aria-describedby="describedBy(!!modelErrors[m.key] && `${ids.models}-${m.key}`)">
                  </span>
                </label>
                <AppButton variant="ghost" size="sm" square class="ms-auto sm:ms-0" :disabled="models.length === 1" :aria-label="t('providers.remove_model', { model: m.id || '…' })" :title="t('providers.remove_model', { model: m.id || '…' })" @click="form.removeModel(m.key)">
                  <Icon name="lucide:x" size="14" aria-hidden="true" />
                </AppButton>
              </div>
            </div>
            <FieldError :id="`${ids.models}-${m.key}`" :message="modelErrors[m.key]" />
            <div class="flex flex-wrap gap-x-4 gap-y-1">
              <label class="flex items-center gap-2 text-xs">
                <input v-model="m.vision" type="checkbox" class="checkbox checkbox-sm rounded-sm" :aria-label="t('providers.vision_for', { model: m.id || '…' })">
                {{ t('providers.vision') }}
              </label>
              <label class="flex items-center gap-2 text-xs">
                <input v-model="m.embedding" type="checkbox" class="checkbox checkbox-sm rounded-sm" :aria-label="t('providers.embedding_for', { model: m.id || '…' })">
                {{ t('providers.embedding') }}
              </label>
            </div>
            <details class="collapse collapse-arrow rounded-box bg-base-200" :open="openPrices.has(m.key) || undefined">
              <summary class="collapse-title min-h-0 py-2 text-xs font-medium">
                {{ t('providers.more_prices') }}
              </summary>
              <div class="collapse-content flex flex-wrap gap-2">
                <label v-for="k in morePriceKeys" :key="k" class="flex w-28 flex-col gap-1">
                  <span class="label text-xs text-base-content">{{ priceLabels[k] }}</span>
                  <span class="input input-sm w-full tabular-nums">
                    <span aria-hidden="true">$</span>
                    <input v-model="m.prices[k]" type="text" inputmode="decimal" :aria-label="t('providers.price_for', { kind: priceLabels[k], model: m.id || '…' })" :aria-describedby="describedBy(!!modelErrors[m.key] && `${ids.models}-${m.key}`)">
                  </span>
                </label>
              </div>
            </details>
          </li>
        </ul>
        <p class="text-xs text-base-content/70">
          {{ t('providers.price_unit') }}
        </p>
        <div>
          <AppButton variant="ghost" size="sm" :disabled="models.length >= maxModels" @click="form.addModel()">
            <Icon name="lucide:plus" size="16" aria-hidden="true" />
            {{ t('providers.add_model') }}
          </AppButton>
        </div>
      </fieldset>
    </form>

    <template #footer>
      <AppButton variant="plain" size="sm" @click="form.hide()">
        {{ readOnly ? t('common.close') : t('common.cancel') }}
      </AppButton>
      <AppButton v-if="!readOnly" size="sm" :busy="busy" @click="form.submit()">
        {{ t('providers.save') }}
      </AppButton>
    </template>
  </AppModal>
</template>
