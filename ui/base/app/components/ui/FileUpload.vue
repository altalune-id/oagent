<script setup lang="ts">
import type { UploadItem, UploadReject, UploadState } from './upload';
import { computed, ref, useId } from 'vue';
import { acceptsFile } from './upload';

const { label, hint, drop, queue, states, accept, maxBytes, items, disabled = false } = defineProps<{
  label: string,
  hint: string,
  drop: string,
  queue: string,
  states: Record<UploadState, string>,
  accept: string,
  maxBytes: number,
  items: UploadItem[],
  disabled?: boolean,
}>();

const emit = defineEmits<{ select: [files: File[]], reject: [rejects: UploadReject[]] }>();

const hintId = `${useId()}-hint`;
const over = ref(false);
const settled = computed(() => items.filter(i => i.state === 'done' || i.state === 'failed'));

const icons: Record<UploadState, string> = {
  queued: 'lucide:clock',
  uploading: 'lucide:upload',
  done: 'lucide:circle-check',
  failed: 'lucide:circle-alert',
};
const tones: Record<UploadState, string> = {
  queued: 'text-base-content/70',
  uploading: 'text-info',
  done: 'text-success',
  failed: 'text-error',
};

function percent(item: UploadItem): number {
  return Math.round(item.progress * 100);
}

function take(files: File[]) {
  const rejects: UploadReject[] = [];
  const ok = files.filter((file) => {
    const reason = !acceptsFile(file, accept) ? 'type' : file.size > maxBytes ? 'size' : '';
    if (reason) {
      rejects.push({ file, reason });
    }
    return !reason;
  });
  if (ok.length) {
    emit('select', ok);
  }
  if (rejects.length) {
    emit('reject', rejects);
  }
}

function pick(e: Event) {
  const input = e.target as HTMLInputElement;
  const files = [...(input.files ?? [])];
  input.value = '';
  take(files);
}

function dropped(e: DragEvent) {
  over.value = false;
  if (!disabled) {
    take([...(e.dataTransfer?.files ?? [])]);
  }
}
</script>

<template>
  <div class="space-y-3">
    <div
      data-dropzone
      class="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-4 sm:rounded-box sm:border sm:border-dashed sm:p-4"
      :class="over && !disabled ? 'sm:border-primary sm:bg-base-200' : 'sm:border-base-300'"
      @dragover.prevent="over = true"
      @dragleave="over = false"
      @drop.prevent="dropped"
    >
      <label class="btn btn-sm w-full text-ui has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 sm:w-auto" :class="{ 'btn-disabled': disabled }">
        <Icon name="lucide:file-up" size="16" class="shrink-0" aria-hidden="true" />
        {{ label }}
        <input
          type="file"
          multiple
          class="sr-only"
          :accept="accept || undefined"
          :disabled="disabled"
          :aria-describedby="hintId"
          @change="pick"
        >
      </label>
      <p class="text-xs text-base-content/70">
        <span :id="hintId">{{ hint }}</span> <span class="hidden sm:inline" aria-hidden="true">{{ drop }}</span>
      </p>
    </div>
    <ul v-if="items.length" class="list rounded-box border border-base-300 text-ui" :aria-label="queue">
      <li v-for="item in items" :key="item.key" class="list-row grid-cols-1 gap-1 py-2">
        <div class="flex min-w-0 items-baseline gap-2">
          <span data-name class="min-w-0 flex-1 truncate font-medium" :title="item.name">{{ item.name }}</span>
          <span class="shrink-0 text-xs text-base-content/70 tabular-nums">{{ item.size }}</span>
        </div>
        <div class="flex items-center gap-2">
          <template v-if="item.state === 'uploading'">
            <progress class="progress min-w-0 flex-1" :value="percent(item)" max="100" :aria-label="item.name" />
            <span data-percent class="w-10 shrink-0 text-end text-xs tabular-nums">{{ percent(item) }}%</span>
          </template>
          <span class="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
            <Icon :name="icons[item.state]" size="14" class="shrink-0" :class="tones[item.state]" aria-hidden="true" />
            <span data-state>{{ states[item.state] }}</span>
          </span>
        </div>
        <p v-if="item.error" class="text-xs text-base-content/70">
          {{ item.error }}
        </p>
      </li>
    </ul>
    <ul class="sr-only" aria-live="polite">
      <li v-for="item in settled" :key="item.key">
        {{ item.name }}: {{ states[item.state] }}{{ item.error ? `. ${item.error}` : '' }}
      </li>
    </ul>
  </div>
</template>
