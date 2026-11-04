<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import Section from '#base/components/ui/Section.vue';

export interface ProfilePasswordLabels {
  title: string;
  email: string;
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
  <Section :title="labels.title">
    <form novalidate class="flex flex-col gap-3" @submit.prevent="emit('submit')">
      <label class="hidden">
        <span>{{ labels.email }}</span>
        <input type="email" name="email" :value="email" autocomplete="username" readonly>
      </label>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ labels.current }}</span>
          <input
            v-model="current" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.current_password }" type="password" name="current-password" autocomplete="current-password" required
            :aria-invalid="!!fieldErrors.current_password" aria-describedby="current-password-error"
          >
        </label>
        <FieldError id="current-password-error" :message="fieldErrors.current_password" />
      </div>
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ labels.next }}</span>
          <input
            v-model="next" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.new_password }" type="password" name="new-password" autocomplete="new-password" minlength="12" required
            :aria-invalid="!!fieldErrors.new_password" aria-describedby="new-password-error new-password-rules"
          >
        </label>
        <FieldError id="new-password-error" :message="fieldErrors.new_password" />
      </div>
      <p id="new-password-rules" class="text-xs text-base-content/70">
        {{ labels.rules }}
      </p>
      <ErrorText :error="error" />
      <SavedNote v-if="notice" :label="notice" />
      <div class="flex items-center justify-between">
        <p class="text-xs text-base-content/70">
          {{ labels.hint }}
        </p>
        <AppButton type="submit" size="sm" :disabled="busy">
          {{ labels.change }}
        </AppButton>
      </div>
    </form>
  </Section>
</template>
