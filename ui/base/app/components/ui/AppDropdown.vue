<script setup lang="ts">
import { onMounted, ref, useId } from 'vue';

const { label = '', open = false, disabled = false, placement = 'start', triggerClass = '', contentClass = 'w-56', bare = false } = defineProps<{
  label?: string,
  open?: boolean,
  disabled?: boolean,
  placement?: 'start' | 'end' | 'top',
  triggerClass?: string,
  bare?: boolean,
  contentClass?: string,
}>();

const placements = { start: 'dropdown-start mt-1', end: 'dropdown-end mt-1', top: 'dropdown-top mb-1' } as const;
const id = `dd-${useId()}`;
const anchor = `--${id}`;
const panel = ref<HTMLElement>();
const expanded = ref(false);

function close() {
  panel.value?.hidePopover?.();
}

function onToggle(event: Event) {
  expanded.value = (event as ToggleEvent).newState === 'open';
}

function onClick(event: MouseEvent) {
  const picked = (event.target as HTMLElement).closest('a, button');
  if (picked && !picked.hasAttribute('data-keep-open')) {
    close();
  }
}

onMounted(() => {
  if (open) {
    panel.value?.showPopover?.();
  }
});

defineExpose({ close });
</script>

<template>
  <div class="inline-flex">
    <button
      type="button"
      class="btn btn-ghost btn-sm"
      :class="triggerClass"
      :disabled="disabled"
      :popovertarget="id"
      aria-haspopup="true"
      :aria-expanded="expanded"
      :style="{ anchorName: anchor }"
    >
      <span v-if="label" class="sr-only">{{ label }}</span>
      <slot name="trigger" />
    </button>
    <div
      :id="id"
      ref="panel"
      popover="auto"
      class="dropdown border border-base-300 bg-base-100 shadow-sm"
      :class="[{ 'rounded-box p-2': !bare }, placements[placement], contentClass]"
      :style="{ positionAnchor: anchor }"
      @toggle="onToggle"
      @click="onClick"
    >
      <slot />
    </div>
  </div>
</template>
