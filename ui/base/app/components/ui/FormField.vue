<script setup lang="ts">
import { computed, useId } from 'vue';
import FieldError from './FieldError.vue';

const { label, hint = '', error = '', required = false, requiredLabel = 'required', hideLabel = false } = defineProps<{
  label: string,
  hint?: string,
  error?: string,
  required?: boolean,
  requiredLabel?: string,
  hideLabel?: boolean,
}>();

const id = useId();
const hintId = `${id}-hint`;
const errorId = `${id}-error`;
const describedBy = computed(() => [hint && hintId, error && errorId].filter(Boolean).join(' ') || undefined);
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1">
    <div class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
      <label :for="id" class="label text-ui font-semibold text-base-content" :class="{ 'sr-only': hideLabel }">
        {{ label }}
        <span v-if="required" class="text-base-content/70">({{ requiredLabel }})</span>
      </label>
      <slot name="label-end" />
    </div>
    <slot :id="id" :described-by="describedBy" :invalid="!!error" />
    <div v-if="hint || $slots['hint-end']" class="flex items-start justify-between gap-2">
      <p v-if="hint" :id="hintId" class="label whitespace-normal text-xs text-base-content/70">
        {{ hint }}
      </p>
      <slot name="hint-end" />
    </div>
    <FieldError :id="errorId" :message="error" />
  </div>
</template>
