<script setup lang="ts">
import { useToast } from './toast';

const { closeLabel = 'Dismiss' } = defineProps<{ closeLabel?: string }>();

const { toasts, dismiss, pause, resume } = useToast();
const icons = { success: 'lucide:circle-check', info: 'lucide:info', error: 'lucide:circle-alert' } as const;
</script>

<template>
  <div class="toast toast-end toast-bottom z-50" aria-live="polite">
    <div
      v-for="item in toasts"
      :key="item.id"
      :role="item.kind === 'error' ? 'alert' : 'status'"
      class="alert"
      :class="`alert-${item.kind}`"
      @mouseenter="pause(item.id)"
      @mouseleave="resume(item.id)"
      @focusin="pause(item.id)"
      @focusout="resume(item.id)"
    >
      <Icon :name="icons[item.kind]" size="18" />
      <span>{{ item.text }}</span>
      <button type="button" class="btn btn-ghost btn-xs btn-square" :aria-label="closeLabel" @click="dismiss(item.id)">
        <Icon name="lucide:x" size="14" />
      </button>
    </div>
  </div>
</template>
