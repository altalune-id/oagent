<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import Section from '#base/components/ui/Section.vue';

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
  <Section :title="labels.title">
    <form class="flex flex-col gap-3" @submit.prevent="emit('save')">
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
      <div class="flex justify-end">
        <AppButton type="submit" size="sm" :disabled="busy">
          {{ labels.save }}
        </AppButton>
      </div>
    </form>
  </Section>
</template>
