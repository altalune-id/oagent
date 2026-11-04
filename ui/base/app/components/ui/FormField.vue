<script setup lang="ts">
import { computed, useId } from 'vue';
import FieldError from './FieldError.vue';

const { label, hint = '', error = '', required = false, requiredLabel = 'required' } = defineProps<{
  label: string,
  hint?: string,
  error?: string,
  required?: boolean,
  requiredLabel?: string,
}>();

const id = useId();
const hintId = `${id}-hint`;
const errorId = `${id}-error`;
const describedBy = computed(() => [hint && hintId, error && errorId].filter(Boolean).join(' ') || undefined);
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1">
    <label :for="id" class="label text-ui font-semibold text-base-content">
      {{ label }}
      <span v-if="required" class="text-base-content/70">({{ requiredLabel }})</span>
    </label>
    <slot :id="id" :described-by="describedBy" :invalid="!!error" />
    <p v-if="hint" :id="hintId" class="label whitespace-normal text-xs text-base-content/70">
      {{ hint }}
    </p>
    <FieldError :id="errorId" :message="error" />
  </div>
</template>
