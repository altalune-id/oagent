<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';

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
      <div class="flex flex-col gap-1">
        <label class="flex flex-col gap-1">
          <span class="label text-ui text-base-content">{{ labels.name }}</span>
          <input
            v-model="name" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldError }" type="text" name="name" maxlength="200" autocomplete="name"
            :aria-invalid="!!fieldError" aria-describedby="profile-name-error"
          >
        </label>
        <FieldError id="profile-name-error" :message="fieldError" />
      </div>
      <p class="text-xs text-base-content/70">
        {{ labels.hint }}
      </p>
      <ErrorText :error="error" />
      <SavedNote v-if="notice" :label="notice" />
      <div class="card-actions justify-end">
        <AppButton type="submit" size="sm" :disabled="busy">
          {{ labels.save }}
        </AppButton>
      </div>
    </div>
  </form>
</template>
