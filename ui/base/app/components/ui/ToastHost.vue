<script setup lang="ts">
import { useToast } from './toast';

const { closeLabel = 'Dismiss' } = defineProps<{ closeLabel?: string }>();

const { toasts, dismiss, pause, resume } = useToast();
const kinds = {
  success: { icon: 'lucide:circle-check', tone: 'text-success' },
  info: { icon: 'lucide:info', tone: 'text-info' },
  error: { icon: 'lucide:circle-alert', tone: 'text-error' },
} as const;
</script>

<template>
  <div class="toast toast-end toast-bottom z-50" role="status" aria-live="polite">
    <div
      v-for="item in toasts"
      :key="item.id"
      class="alert text-ui shadow-sm"
      @mouseenter="pause(item.id)"
      @mouseleave="resume(item.id)"
      @focusin="pause(item.id)"
      @focusout="resume(item.id)"
    >
      <Icon :name="kinds[item.kind].icon" size="16" class="shrink-0" :class="kinds[item.kind].tone" aria-hidden="true" />
      <span>{{ item.text }}</span>
      <button type="button" class="btn btn-ghost btn-xs btn-square" :aria-label="closeLabel" @click="dismiss(item.id)">
        <Icon name="lucide:x" size="14" />
      </button>
    </div>
  </div>
</template>
