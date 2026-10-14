<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';
import ErrorText from '../ui/ErrorText.vue';
import FieldError from '../ui/FieldError.vue';

export interface ProfileNameLabels {
  title: string;
  name: string;
  hint: string;
  save: string;
}

const { busy = false, notice = '', error = null, fieldError = '', labels } = defineProps<{
  busy?: boolean,
  notice?: string,
  error?: ShownError | null,
  fieldError?: string,
  labels: ProfileNameLabels,
}>();
const emit = defineEmits<{ save: [] }>();
const name = defineModel<string>('name', { required: true });
</script>

<template>
  <form class="card border border-base-300 bg-base-100" @submit.prevent="emit('save')">
    <div class="card-body gap-3">
      <h2 class="card-title text-base">
        {{ labels.title }}
      </h2>
      <label class="floating-label">
        <span>{{ labels.name }}</span>
        <input
          v-model="name" class="input w-full" :class="{ 'input-error': fieldError }" type="text" name="name" maxlength="200" autocomplete="name"
          :placeholder="labels.name" :aria-invalid="!!fieldError" aria-describedby="profile-name-error"
        >
        <FieldError id="profile-name-error" :message="fieldError" />
      </label>
      <p class="text-xs text-base-content/60">
        {{ labels.hint }}
      </p>
      <ErrorText :error="error" />
      <p v-if="notice" role="status" class="text-sm text-success">
        {{ notice }}
      </p>
      <div class="card-actions justify-end">
        <button type="submit" class="btn btn-primary btn-sm" :disabled="busy">
          {{ labels.save }}
        </button>
      </div>
    </div>
  </form>
</template>
