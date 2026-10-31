<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';

export interface ProfilePasswordLabels {
  title: string;
  current: string;
  next: string;
  rules: string;
  hint: string;
  change: string;
}

const { email = '', busy = false, notice = '', error = null, fieldErrors = {}, labels } = defineProps<{
  email?: string,
  busy?: boolean,
  notice?: string,
  error?: ShownError | null,
  // fieldErrors are keyed by request field: current_password, new_password.
  fieldErrors?: Record<string, string>,
  labels: ProfilePasswordLabels,
}>();
const emit = defineEmits<{ submit: [] }>();
const current = defineModel<string>('current', { required: true });
const next = defineModel<string>('next', { required: true });
</script>

<template>
  <form novalidate class="card border border-base-300 bg-base-100" @submit.prevent="emit('submit')">
    <div class="card-body gap-3">
      <h2 class="card-title text-base">
        {{ labels.title }}
      </h2>
      <input type="email" name="email" :value="email" autocomplete="username" class="hidden" readonly>
      <label class="floating-label">
        <span>{{ labels.current }}</span>
        <input
          v-model="current" class="input w-full" :class="{ 'input-error': fieldErrors.current_password }" type="password" name="current-password" autocomplete="current-password" required
          :placeholder="labels.current" :aria-invalid="!!fieldErrors.current_password" aria-describedby="current-password-error"
        >
        <FieldError id="current-password-error" :message="fieldErrors.current_password" />
      </label>
      <label class="floating-label">
        <span>{{ labels.next }}</span>
        <input
          v-model="next" class="input w-full" :class="{ 'input-error': fieldErrors.new_password }" type="password" name="new-password" autocomplete="new-password" minlength="12" required
          :placeholder="labels.next" :aria-invalid="!!fieldErrors.new_password" aria-describedby="new-password-error new-password-rules"
        >
        <FieldError id="new-password-error" :message="fieldErrors.new_password" />
      </label>
      <p id="new-password-rules" class="text-xs text-base-content/60">
        {{ labels.rules }}
      </p>
      <ErrorText :error="error" />
      <SavedNote v-if="notice" :label="notice" />
      <div class="card-actions items-center justify-between">
        <p class="text-xs text-base-content/60">
          {{ labels.hint }}
        </p>
        <button type="submit" class="btn btn-primary btn-sm" :disabled="busy">
          {{ labels.change }}
        </button>
      </div>
    </div>
  </form>
</template>
