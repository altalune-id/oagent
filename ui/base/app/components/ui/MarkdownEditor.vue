<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue';
import { computed, nextTick, ref, useId } from 'vue';
import { renderMarkdown } from '../../lib/format/markdown';
import { codePoints } from '../../lib/format/text';
import AppButton from './AppButton.vue';
import AppModal from './AppModal.vue';
import { focusWhenShown } from './focusWhenShown';
import FormField from './FormField.vue';
import { countTone } from './markdownEditor';
import MarkdownEditor from './MarkdownEditor.vue';

interface Labels {
  write: string;
  preview: string;
  done: string;
  empty: string;
}

const { label, max, hint = '', error = '', rows = 6, placeholder = '', readonly = false, bare = false, labels = { write: 'Write', preview: 'Preview', done: 'Done', empty: 'Nothing to preview' }, expandLabel = '', viewLabel = '' } = defineProps<{
  label: string,
  max: number,
  hint?: string,
  error?: string,
  rows?: 3 | 6,
  placeholder?: string,
  readonly?: boolean,
  bare?: boolean,
  labels?: Labels,
  expandLabel?: string,
  viewLabel?: string,
}>();

const model = defineModel<string>({ required: true });

const uid = useId();
const ids = { write: `${uid}-write`, preview: `${uid}-preview`, panel: `${uid}-panel`, count: `${uid}-count` };
const tab = ref<'write' | 'preview'>('write');
const tabs = computed(() => [
  { key: 'write' as const, id: ids.write, label: labels.write },
  { key: 'preview' as const, id: ids.preview, label: labels.preview },
]);
const showPreview = computed(() => readonly || tab.value === 'preview');
const html = computed(() => (showPreview.value ? renderMarkdown(model.value) : ''));
const count = computed(() => codePoints(model.value));
const tone = computed(() => countTone(count.value, max));
const invalid = computed(() => !!error || tone.value === 'error');
const toneClass = { ok: 'text-base-content/70', warning: 'text-warning', error: 'text-error' };
const minHeight = { 3: 'min-h-[calc(3lh+1rem)]', 6: 'min-h-[calc(6lh+1rem)]' }[rows];

const expanded = ref(false);
const expandButton = ref<ComponentPublicInstance>();
const modalBody = ref<HTMLElement>();
const inner = ref<{ field?: HTMLTextAreaElement }>();
const textarea = ref<HTMLTextAreaElement>();

function describedBy(field?: string) {
  return [field, ids.count].filter(Boolean).join(' ');
}

async function select(key: 'write' | 'preview', focus = false) {
  tab.value = key;
  if (focus) {
    await nextTick();
    document.getElementById(key === 'write' ? ids.write : ids.preview)?.focus();
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
    e.preventDefault();
    void select(tab.value === 'write' ? 'preview' : 'write', true);
  }
}

async function expand() {
  expanded.value = true;
  await nextTick();
  focusWhenShown(() => modalBody.value, () => inner.value?.field, () => expanded.value);
}

async function collapse() {
  expanded.value = false;
  await nextTick();
  (expandButton.value?.$el as HTMLElement | undefined)?.focus();
}

defineExpose({ field: textarea });
</script>

<template>
  <FormField :label="label" :hint="bare ? '' : hint" :error="error" :hide-label="bare">
    <template v-if="!readonly" #label-end>
      <div class="ms-auto flex items-center gap-1">
        <div role="tablist" class="tabs tabs-border tabs-xs" :aria-label="viewLabel || `${label} view`">
          <button
            v-for="t in tabs" :id="t.id" :key="t.key" type="button" role="tab" class="tab"
            :class="{ 'tab-active': tab === t.key }" :aria-selected="tab === t.key" :aria-controls="ids.panel"
            :tabindex="tab === t.key ? 0 : -1" @click="select(t.key)" @keydown="onKey"
          >
            {{ t.label }}
          </button>
        </div>
        <AppButton v-if="!bare" ref="expandButton" variant="ghost" size="xs" square :aria-label="expandLabel || `Expand ${label}`" @click="expand()">
          <Icon name="lucide:maximize-2" size="14" aria-hidden="true" />
        </AppButton>
      </div>
    </template>
    <template #default="field">
      <div :id="ids.panel" :role="readonly ? undefined : 'tabpanel'" :aria-labelledby="readonly ? undefined : (tab === 'write' ? ids.write : ids.preview)">
        <textarea
          v-if="!showPreview" :id="field.id" ref="textarea" v-model="model" :rows="rows" :placeholder="placeholder"
          class="textarea textarea-sm w-full overflow-y-auto text-ui leading-relaxed field-sizing-content"
          :class="[minHeight, bare ? 'max-h-[70vh]' : 'max-h-[60vh]', { 'textarea-error': invalid }]"
          :aria-invalid="invalid" :aria-describedby="describedBy(field.describedBy)"
        />
        <div
          v-else-if="!model.trim()" class="flex items-center justify-center rounded-box border border-base-300 bg-base-100 p-3 text-ui leading-relaxed text-base-content/70"
          :class="minHeight"
        >
          {{ labels.empty }}
        </div>
        <!-- SECURITY: renderMarkdown escapes raw HTML and drops script links, so v-html only shows markdown output. -->
        <div
          v-else
          class="overflow-auto rounded-box border border-base-300 bg-base-100 p-3 text-ui leading-relaxed [&_blockquote]:border-s-2 [&_blockquote]:border-base-300 [&_blockquote]:ps-3 [&_blockquote]:text-base-content/70 [&_code]:rounded [&_code]:bg-base-200 [&_code]:px-1 [&_code]:font-mono [&_code]:text-xs [&_h1]:text-base [&_h1]:font-semibold [&_h2]:text-ui [&_h2]:font-semibold [&_h3]:text-ui [&_h3]:font-semibold [&_hr]:border-base-300 [&_ol]:[list-style-type:decimal] [&_ol]:ps-5 [&_pre]:overflow-x-auto [&_pre]:rounded-box [&_pre]:bg-base-200 [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_ul]:[list-style-type:disc] [&_ul]:ps-5 [&>*+*]:mt-2"
          :class="[minHeight, bare ? 'max-h-[70vh]' : 'max-h-[60vh]']"
          v-html="html"
        />
      </div>
    </template>
    <template #hint-end>
      <span :id="ids.count" class="ms-auto shrink-0 text-xs tabular-nums" :class="toneClass[tone]">{{ count }} / {{ max }}</span>
    </template>
  </FormField>

  <AppModal v-if="!bare && !readonly" :open="expanded" :title="label" wide @close="collapse()">
    <div ref="modalBody">
      <MarkdownEditor v-if="expanded" ref="inner" v-model="model" bare :label="label" :max="max" :error="error" :rows="rows" :placeholder="placeholder" :labels="labels" :view-label="viewLabel" />
    </div>
    <template #footer>
      <AppButton size="sm" @click="collapse()">
        {{ labels.done }}
      </AppButton>
    </template>
  </AppModal>
</template>
